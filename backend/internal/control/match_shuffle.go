package control

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"sort"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

type shufflePlayer struct {
	SteamID string `json:"steamId"`
	Side    string `json:"side"`
	Ready   bool   `json:"ready"`
}

func (a *API) shuffleMatch(w http.ResponseWriter, r *http.Request) {
	a.shuffleMatchFor(w, r, "")
}

func (a *API) playerShuffleMatch(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if ok {
		a.shuffleMatchFor(w, r, player.SteamID)
	}
}

func (a *API) rosterFingerprint(room string, players []shufflePlayer) string {
	sort.Slice(players, func(i, j int) bool { return players[i].SteamID < players[j].SteamID })
	encoded, _ := json.Marshal(players)
	mac := hmac.New(sha256.New, []byte(a.AdminToken))
	mac.Write([]byte(room))
	mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *API) participantRemovalToken(room, steam string) string {
	mac := hmac.New(sha256.New, []byte(a.AdminToken))
	mac.Write([]byte("room-participant-removal\x00" + room + "\x00" + steam))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *API) shuffleMatchFor(w http.ResponseWriter, r *http.Request, steam string) {
	var in struct {
		SettingsRevision  *int64          `json:"settingsRevision"`
		Players           []shufflePlayer `json:"players"`
		RosterFingerprint string          `json:"rosterFingerprint"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.SettingsRevision == nil || (steam == "" && (len(in.Players) < 2 || len(in.Players) > 10)) || (steam != "" && len(in.RosterFingerprint) != 64) {
		fail(w, 400, "Provide the current room settings and 2 to 10 players")
		return
	}
	ids := make([]string, 0, len(in.Players))
	expected := make(map[string]shufflePlayer, len(in.Players))
	for _, p := range in.Players {
		ids = append(ids, p.SteamID)
		if p.Side != "ct" && p.Side != "t" {
			fail(w, 400, "Invalid roster")
			return
		}
		expected[p.SteamID] = p
	}
	if steam == "" && protocol.ValidateWhitelist(ids) != nil {
		fail(w, 400, "Invalid roster")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var locked string
	if steam != "" {
		err = tx.QueryRow(ctx, `SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF h`, r.PathValue("id")).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Room unavailable for player management")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
	}
	err = tx.QueryRow(ctx, `SELECT m.id FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF m`, r.PathValue("id")).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if steam != "" {
		allowed, e := a.playerRoomPermission(ctx, tx, locked, steam, false)
		if e != nil {
			internal(w, e)
			return
		}
		if !allowed {
			fail(w, 404, "Room unavailable for player management")
			return
		}
	}
	var status, load string
	var revision int64
	var size int
	var tournament bool
	err = tx.QueryRow(ctx, `SELECT status,load_status,settings_revision,players_per_side,EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=match_rooms.id) FROM match_rooms WHERE id=$1`, locked).Scan(&status, &load, &revision, &size, &tournament)
	if err != nil {
		internal(w, err)
		return
	}
	if status != "open" || load != "" || tournament || revision != *in.SettingsRevision {
		fail(w, 409, "The room changed or its roster is locked")
		return
	}
	rows, err := tx.Query(ctx, `SELECT steam_id,side,ready FROM match_room_players WHERE match_id=$1 ORDER BY steam_id`, locked)
	if err != nil {
		internal(w, err)
		return
	}
	players := []shufflePlayer{}
	for rows.Next() {
		var p shufflePlayer
		if err = rows.Scan(&p.SteamID, &p.Side, &p.Ready); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		players = append(players, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	if len(players) < 2 || len(players) > 2*size || (steam == "" && len(players) != len(in.Players)) || (steam != "" && !hmac.Equal([]byte(a.rosterFingerprint(locked, players)), []byte(in.RosterFingerprint))) {
		fail(w, 409, "The room roster changed; review it before shuffling")
		return
	}
	for _, p := range players {
		if steam != "" {
			break
		}
		if previous, ok := expected[p.SteamID]; !ok || previous != p {
			fail(w, 409, "The room roster changed; review it before shuffling")
			return
		}
	}
	for i := len(players) - 1; i > 0; i-- {
		n, e := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if e != nil {
			internal(w, e)
			return
		}
		j := int(n.Int64())
		players[i], players[j] = players[j], players[i]
	}
	ctCount := len(players) / 2
	if len(players)%2 != 0 {
		extra, e := rand.Int(rand.Reader, big.NewInt(2))
		if e != nil {
			internal(w, e)
			return
		}
		ctCount += int(extra.Int64())
	}
	ct := make([]string, 0, ctCount)
	for _, p := range players[:ctCount] {
		ct = append(ct, p.SteamID)
	}
	if _, err = tx.Exec(ctx, `UPDATE match_room_players SET side=CASE WHEN steam_id=ANY($2::text[]) THEN 'ct' ELSE 't' END,ready=false WHERE match_id=$1`, locked, ct); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
