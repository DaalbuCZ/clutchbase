package control

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type cachedJoinPassword struct {
	value    string
	received time.Time
}

const joinPasswordFreshness = 15 * time.Second

func (a *API) rememberJoinPassword(id, password string) {
	a.joinPasswordsMu.Lock()
	defer a.joinPasswordsMu.Unlock()
	if a.joinPasswords == nil {
		a.joinPasswords = make(map[string]cachedJoinPassword)
	}
	if id == "" || password == "" {
		delete(a.joinPasswords, id)
		return
	}
	a.joinPasswords[id] = cachedJoinPassword{value: password, received: time.Now()}
}

func (a *API) cachedJoinPassword(id string) (string, bool) {
	a.joinPasswordsMu.RLock()
	entry, ok := a.joinPasswords[id]
	a.joinPasswordsMu.RUnlock()
	if !ok || time.Since(entry.received) >= joinPasswordFreshness {
		return "", false
	}
	return entry.value, true
}

func (a *API) getJoinPassword(w http.ResponseWriter, r *http.Request) {
	var machineID *string
	var lastSeen *time.Time
	var driver string
	err := a.DB.QueryRow(r.Context(), `SELECT machine_id,last_seen,driver FROM hosts WHERE id=$1 AND removed_at IS NULL`, r.PathValue("id")).Scan(&machineID, &lastSeen, &driver)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if machineID == nil || driver != "managed" {
		fail(w, http.StatusConflict, "Join passwords are available for managed machine servers")
		return
	}
	if lastSeen == nil || time.Since(*lastSeen) >= 20*time.Second {
		fail(w, http.StatusConflict, "Machine is offline; reconnect it to retrieve the join password")
		return
	}
	password, ok := a.cachedJoinPassword(r.PathValue("id"))
	if !ok {
		fail(w, http.StatusConflict, "The connected agent has not reported the join password yet")
		return
	}
	respond(w, http.StatusOK, map[string]string{"password": password})
}
