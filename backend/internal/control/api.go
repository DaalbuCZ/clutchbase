package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

type API struct {
	DB         *pgxpool.Pool
	AdminToken string
}

func (a *API) Migrate(ctx context.Context) error { _, err := a.DB.Exec(ctx, schema); return err }

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.DB.Ping(r.Context()); err != nil {
			fail(w, 503, "Database unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/state", a.admin(a.list))
	mux.HandleFunc("POST /api/hosts", a.admin(a.create))
	mux.HandleFunc("PATCH /api/hosts/{id}", a.admin(a.update))
	mux.HandleFunc("POST /api/agent/{id}/heartbeat", a.heartbeat)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "API endpoint not found") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(digest(a)), []byte(digest(b))) == 1
}
func bearer(r *http.Request) string {
	value, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return value
}
func (a *API) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.AdminToken == "" || !equal(bearer(r), a.AdminToken) {
			fail(w, 401, "Administrator token required")
			return
		}
		next(w, r)
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "Invalid JSON request")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Print("Response write failed")
	}
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func internal(w http.ResponseWriter, err error) {
	log.Printf("Database operation failed: %v", err)
	fail(w, 500, "Database operation failed")
}

const columns = "id, name, address, port, map, desired_state, observed_state, active_map, driver, error, last_seen, revision"

func scan(row pgx.Row) (protocol.Host, error) {
	var h protocol.Host
	err := row.Scan(&h.ID, &h.Name, &h.Address, &h.Port, &h.Map, &h.DesiredState, &h.ObservedState, &h.ActiveMap, &h.Driver, &h.Error, &h.LastSeen, &h.Revision)
	h.Project(time.Now())
	return h, err
}
func (a *API) list(w http.ResponseWriter, r *http.Request) {
	capturedAt := time.Now().UTC()
	rows, err := a.DB.Query(r.Context(), "SELECT "+columns+" FROM hosts ORDER BY created_at, id")
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	hosts := []protocol.Host{}
	for rows.Next() {
		h, err := scan(rows)
		if err != nil {
			internal(w, err)
			return
		}
		hosts = append(hosts, h)
	}
	if err := rows.Err(); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, protocol.Workspace{CapturedAt: capturedAt.Format("2006-01-02T15:04:05.000000000Z"), Hosts: hosts, Maps: protocol.Maps})
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,252}$`)

func validAddress(s string) bool { return net.ParseIP(s) != nil || hostname.MatchString(s) }
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Port    int    `json:"port"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	if in.Name == "" || len(in.Name) > 80 || !validAddress(in.Address) || in.Port < 1 || in.Port > 65535 {
		fail(w, 400, "Provide a name, hostname/IP, and port between 1 and 65535")
		return
	}
	id := rand.Text()
	token := rand.Text() + rand.Text()
	h, err := scan(a.DB.QueryRow(r.Context(), "INSERT INTO hosts (id,name,address,port,token_hash) VALUES ($1,$2,$3,$4,$5) RETURNING "+columns, id, in.Name, in.Address, in.Port, digest(token)))
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, map[string]any{"host": h, "agentToken": token})
}

func (a *API) update(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Map      *string `json:"map"`
		State    *string `json:"desiredState"`
		Revision *int64  `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Map == nil && in.State == nil {
		fail(w, 400, "No changes supplied")
		return
	}
	if in.Map != nil && !protocol.ValidMap(*in.Map) {
		fail(w, 400, "Unsupported map")
		return
	}
	if in.State != nil && *in.State != "running" && *in.State != "stopped" {
		fail(w, 400, "Unsupported state")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	h, err := scan(tx.QueryRow(r.Context(), "SELECT "+columns+" FROM hosts WHERE id=$1 FOR UPDATE", r.PathValue("id")))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Host not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !h.Online {
		fail(w, 409, "Host is offline; connect its agent first")
		return
	}
	if in.Revision == nil || *in.Revision != h.Revision {
		fail(w, 409, "Server state changed; refresh and try again")
		return
	}
	if in.Map != nil && *in.Map != h.Map {
		if !h.Actions.ChangeMap {
			fail(w, 409, "Stop the server before changing its map")
			return
		}
		h.Map = *in.Map
	}
	if in.State != nil && *in.State != h.DesiredState {
		if (*in.State == "running" && !h.Actions.Start) || (*in.State == "stopped" && !h.Actions.Stop) {
			fail(w, 409, "Action is unavailable in the current server state")
			return
		}
		h.DesiredState = *in.State
	}
	_, err = tx.Exec(r.Context(), "UPDATE hosts SET map=$2, desired_state=$3, revision=revision+1 WHERE id=$1", h.ID, h.Map, h.DesiredState)
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	h.Revision++
	h.Project(time.Now())
	respond(w, 200, h)
}

func (a *API) heartbeat(w http.ResponseWriter, r *http.Request) {
	var hash string
	err := a.DB.QueryRow(r.Context(), "SELECT token_hash FROM hosts WHERE id=$1", r.PathValue("id")).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "Invalid agent credentials")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(digest(bearer(r))), []byte(hash)) != 1 {
		fail(w, 401, "Invalid agent credentials")
		return
	}
	var in protocol.Report
	if !decode(w, r, &in) {
		return
	}
	if (in.State != "running" && in.State != "stopped" && in.State != "error" && in.State != "starting" && in.State != "stopping") || (in.Driver != "simulated" && in.Driver != "systemd") || (in.ActiveMap != "" && !protocol.ValidMap(in.ActiveMap)) || len(in.Error) > 1000 {
		fail(w, 400, "Invalid agent status")
		return
	}
	var desired protocol.Desired
	err = a.DB.QueryRow(r.Context(), "UPDATE hosts SET observed_state=$2,active_map=$3,driver=$4,error=$5,last_seen=now() WHERE id=$1 RETURNING desired_state,map", r.PathValue("id"), in.State, in.ActiveMap, in.Driver, in.Error).Scan(&desired.State, &desired.Map)
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, desired)
}
