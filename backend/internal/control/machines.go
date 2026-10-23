package control

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func validReport(in protocol.Report) bool {
	return (in.State == "running" || in.State == "stopped" || in.State == "error" || in.State == "starting" || in.State == "stopping" || in.State == "uninstalled" || in.State == "installing") && (in.Driver == "simulated" || in.Driver == "systemd" || in.Driver == "managed") && (in.ActiveMap == "" || protocol.ValidMap(in.ActiveMap)) && len(in.Error) <= 1000
}

func (a *API) machines(ctx context.Context) ([]protocol.Machine, error) {
	rows, err := a.DB.Query(ctx, `SELECT id,name,address,driver,last_seen,(SELECT count(*) FROM hosts WHERE machine_id=machines.id) FROM machines ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	machines := []protocol.Machine{}
	for rows.Next() {
		var m protocol.Machine
		var count int
		if err := rows.Scan(&m.ID, &m.Name, &m.Address, &m.Driver, &m.LastSeen, &count); err != nil {
			return nil, err
		}
		m.Online = m.LastSeen != nil && time.Since(*m.LastSeen) < 20*time.Second
		m.CreateServer = m.Online && count < protocol.MaxServers
		machines = append(machines, m)
	}
	return machines, rows.Err()
}

func (a *API) createMachine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	if in.Name == "" || len(in.Name) > 80 || !validAddress(in.Address) {
		fail(w, 400, "Provide a machine name and game hostname/IP")
		return
	}
	m := protocol.Machine{ID: rand.Text(), Name: in.Name, Address: in.Address}
	token := rand.Text() + rand.Text()
	_, err := a.DB.Exec(r.Context(), "INSERT INTO machines (id,name,address,token_hash) VALUES ($1,$2,$3,$4)", m.ID, m.Name, m.Address, digest(token))
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		fail(w, 409, "This game address already has a machine registration")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, map[string]any{"machine": m, "agentToken": token})
}

func (a *API) createServer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Map  string `json:"map"`
		Port int    `json:"port"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 || !protocol.ValidMap(in.Map) || (in.Port != 0 && (in.Port < 1024 || in.Port > 65533)) {
		fail(w, 400, "Provide a server name, supported map and game port between 1024 and 65533, or 0 for automatic allocation")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var address string
	var seen *time.Time
	err = tx.QueryRow(r.Context(), "SELECT address,last_seen FROM machines WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&address, &seen)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Machine not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if seen == nil || time.Since(*seen) >= 20*time.Second {
		fail(w, 409, "Connect the machine agent before creating servers")
		return
	}
	var count int
	if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM hosts WHERE machine_id=$1", r.PathValue("id")).Scan(&count); err != nil {
		internal(w, err)
		return
	}
	if count >= protocol.MaxServers {
		fail(w, 409, "Machine server limit reached")
		return
	}
	rows, err := tx.Query(r.Context(), "SELECT port FROM hosts WHERE machine_id=$1 OR lower(address)=lower($2)", r.PathValue("id"), address)
	if err != nil {
		internal(w, err)
		return
	}
	ports := []int{}
	for rows.Next() {
		var p int
		if err = rows.Scan(&p); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		ports = append(ports, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	available := func(p int) bool {
		for _, used := range ports {
			if p <= used+2 && used <= p+2 {
				return false
			}
		}
		return true
	}
	if in.Port == 0 {
		for p := 27015; p <= 65533; p += 10 {
			if available(p) {
				in.Port = p
				break
			}
		}
	}
	if in.Port == 0 || !available(in.Port) {
		fail(w, 409, "Game, client or TV port overlaps another registered server")
		return
	}
	// Fleet instances have no usable single-instance agent credential.
	h, err := scan(tx.QueryRow(r.Context(), `INSERT INTO hosts (id,name,address,port,token_hash,machine_id,map) VALUES ($1,$2,$3,$4,'',$5,$6) RETURNING `+columns, rand.Text(), in.Name, address, in.Port, r.PathValue("id"), in.Map))
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, h)
}

func (a *API) machineHeartbeat(w http.ResponseWriter, r *http.Request) {
	var hash string
	err := a.DB.QueryRow(r.Context(), "SELECT token_hash FROM machines WHERE id=$1", r.PathValue("id")).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "Invalid machine credentials")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(digest(bearer(r))), []byte(hash)) != 1 {
		fail(w, 401, "Invalid machine credentials")
		return
	}
	var in protocol.MachineReport
	if !decodeLimit(w, r, &in, 131072) {
		return
	}
	if (in.Driver != "managed" && in.Driver != "simulated") || len(in.Servers) > protocol.MaxServers {
		fail(w, 400, "Invalid machine status")
		return
	}
	ids := map[string]bool{}
	for _, server := range in.Servers {
		if ids[server.ID] || server.ID == "" || !validReport(server.Report) || server.Driver != in.Driver {
			fail(w, 400, "Invalid or duplicate server status")
			return
		}
		ids[server.ID] = true
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize snapshots and creation on the same machine; reject the whole report if any ID is foreign.
	if _, err = tx.Exec(r.Context(), "UPDATE machines SET driver=$2,last_seen=now() WHERE id=$1", r.PathValue("id"), in.Driver); err != nil {
		internal(w, err)
		return
	}
	for _, server := range in.Servers {
		result, err := tx.Exec(r.Context(), `UPDATE hosts SET observed_state=$3,active_map=$4,driver=$5,error=$6,last_seen=now() WHERE id=$1 AND machine_id=$2`, server.ID, r.PathValue("id"), server.State, server.ActiveMap, server.Driver, server.Error)
		if err != nil {
			internal(w, err)
			return
		}
		if result.RowsAffected() != 1 {
			fail(w, 400, "Server does not belong to this machine")
			return
		}
	}
	rows, err := tx.Query(r.Context(), "SELECT id,desired_state,map,port FROM hosts WHERE machine_id=$1 ORDER BY created_at,id", r.PathValue("id"))
	if err != nil {
		internal(w, err)
		return
	}
	out := protocol.MachineDesired{Servers: []protocol.ServerDesired{}}
	for rows.Next() {
		var s protocol.ServerDesired
		if err = rows.Scan(&s.ID, &s.State, &s.Map, &s.Port); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		out.Servers = append(out.Servers, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, out)
}
