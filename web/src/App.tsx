import { useEffect, useRef, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { APIError, connection, request } from "./api";
import type { Host, Workspace } from "./api";

function Mark() {
  return (
    <span className="mark" aria-hidden="true">
      c<span>↗</span>
    </span>
  );
}
function mapName(map: string) {
  return map.replace(/^de_/, "").replace(/^./, (s) => s.toUpperCase());
}
const message = (error: unknown) =>
  error instanceof Error ? error.message : "Something went wrong";

export function App() {
  const [token, setToken] = useState(
    () => sessionStorage.getItem("clutchbase-token") || "",
  );
  const [snapshot, setSnapshot] = useState<Workspace | null>(null);
  const session = useRef(token);
  session.current = token;
  const hosts = snapshot?.hosts || [];
  const maps = snapshot?.maps || [];
  const accept = (next: Workspace) =>
    setSnapshot((previous) =>
      !previous || next.capturedAt >= previous.capturedAt ? next : previous,
    );
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [credentials, setCredentials] = useState<{
    host: Host;
    agentToken: string;
  } | null>(null);

  function signOut() {
    session.current = "";
    sessionStorage.removeItem("clutchbase-token");
    setToken("");
    setSnapshot(null);
    setCredentials(null);
    setAdding(false);
  }
  async function refresh() {
    const next = await request<Workspace>(token, "/state");
    if (session.current === token) accept(next);
  }
  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;
    setLoading(true);
    const poll = async () => {
      try {
        const next = await request<Workspace>(token, "/state");
        if (!cancelled) {
          accept(next);
          setError("");
        }
      } catch (err) {
        if (!cancelled) {
          if (err instanceof APIError && err.status === 401) {
            signOut();
            setError(
              "Token rejected. Enter the administrator token configured on the API.",
            );
            return;
          }
          setError(message(err));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
      if (!cancelled) timer = setTimeout(poll, 5000);
    };
    void poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [token]);
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(""), 4000);
    return () => clearTimeout(t);
  }, [notice]);

  async function update(host: Host, patch: Record<string, string>) {
    setBusy(host.id);
    setError("");
    try {
      await request(token, `/hosts/${host.id}`, {
        method: "PATCH",
        body: JSON.stringify({ ...patch, revision: host.revision }),
      });
      await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        try {
          await refresh();
        } catch {
          /* Preserve the original action error. */
        }
      }
      setError(message(err));
    } finally {
      setBusy("");
    }
  }
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setNotice("Copied to clipboard");
    } catch {
      setError(
        "Clipboard access is unavailable. Select and copy the displayed text.",
      );
    }
  }
  async function addHost(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy("create");
    setError("");
    try {
      const result = await request<{ host: Host; agentToken: string }>(
        token,
        "/hosts",
        {
          method: "POST",
          body: JSON.stringify({
            name: form.get("name"),
            address: form.get("address"),
            port: Number(form.get("port")),
          }),
        },
      );
      setCredentials(result);
      setAdding(false);
      await refresh();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy("");
    }
  }

  if (!token)
    return (
      <main className="login">
        <div className="login-card">
          <Mark />
          <h1>Clutchbase</h1>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              const value = String(
                new FormData(event.currentTarget).get("token") || "",
              ).trim();
              if (value) {
                setError("");
                setSnapshot(null);
                setLoading(true);
                sessionStorage.setItem("clutchbase-token", value);
                setToken(value);
              }
            }}
          >
            <label>
              Administrator token
              <input
                name="token"
                type="password"
                required
                autoComplete="off"
                placeholder="Enter your API administrator token"
              />
            </label>
            <button className="primary" type="submit">
              Open control room <span>↗</span>
            </button>
          </form>
          <p className="small muted">
            Use CLUTCHBASE_ADMIN_TOKEN from your local configuration. Saved only
            for this browser tab.
          </p>
          {error && (
            <p className="error" role="alert">
              {error}
            </p>
          )}
        </div>
      </main>
    );

  const running = hosts.filter(
    (h) => h.online && h.observedState === "running",
  ).length;
  const agentCommand = credentials
    ? `CLUTCHBASE_API_URL='${import.meta.env.DEV ? "http://127.0.0.1:8080" : window.location.origin}' \\\nCLUTCHBASE_HOST_ID='${credentials.host.id}' \\\nCLUTCHBASE_AGENT_TOKEN='${credentials.agentToken}' \\\n./bin/clutchbase-agent --simulate`
    : "";

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="brand" href="/" aria-label="Clutchbase home">
          <Mark />
          <span>
            clutchbase<span className="brand-dot">.</span>
          </span>
        </a>
        <div className="nav-item selected">
          <span>▤</span> Servers <span className="count">{hosts.length}</span>
        </div>
        <button className="text-button logout" onClick={signOut}>
          ↪ Sign out
        </button>
      </aside>
      <div className="workspace">
        <main className="content">
          <div className="page-heading">
            <div>
              <h1>Servers</h1>
            </div>
            <button className="primary" onClick={() => setAdding(true)}>
              ＋ Register host
            </button>
          </div>
          <div className="stats">
            <div>
              <span className="stat-label">Hosts</span>
              <strong>{hosts.length.toString().padStart(2, "0")}</strong>
            </div>
            <div>
              <span className="stat-label">Running</span>
              <strong>
                {running.toString().padStart(2, "0")}
                <i className="tiny-dot" />
              </strong>
            </div>
            <div>
              <span className="stat-label">Agents online</span>
              <strong>
                {hosts
                  .filter((h) => h.online)
                  .length.toString()
                  .padStart(2, "0")}
              </strong>
            </div>
          </div>
          {error && (
            <div className="error banner" role="alert">
              {error}
            </div>
          )}
          {loading ? (
            <div className="empty" role="status">
              Connecting to your control room…
            </div>
          ) : hosts.length === 0 ? (
            <div className="empty">
              <div className="empty-icon">＋</div>
              <p className="muted">
                Register a Linux host and connect its agent to manage a server.
              </p>
              <button className="primary" onClick={() => setAdding(true)}>
                Register your first host <span>↗</span>
              </button>
            </div>
          ) : (
            <div className="server-grid">
              {hosts.map((host) => {
                const state = host.status;
                const transitioning =
                  state === "starting" || state === "stopping";
                return (
                  <article className="server-card" key={host.id}>
                    <div className={`map-art ${host.map}`}>
                      <span className="map-grid" />
                      <span className="map-word">{mapName(host.map)}</span>
                      <span
                        className={`badge ${state === "running" ? "online" : ""}`}
                      >
                        <i className="tiny-dot" />
                        {state}
                      </span>
                      <span className="map-code">{host.map}</span>
                    </div>
                    <div className="server-body">
                      <div className="server-title">
                        <h2>{host.name}</h2>
                        {host.driver === "simulated" && (
                          <span className="simulation">SIMULATED</span>
                        )}
                      </div>
                      <p className="connection">
                        <code>{connection(host)}</code>
                        <button
                          aria-label={`Copy connection for ${host.name}`}
                          className="text-button"
                          onClick={() =>
                            void copy(`connect ${connection(host)}`)
                          }
                        >
                          Copy ↗
                        </button>
                      </p>
                      <label className="map-label">
                        STARTUP MAP
                        <select
                          aria-label={`Startup map for ${host.name}`}
                          value={host.map}
                          disabled={
                            !host.actions.changeMap ||
                            busy === host.id ||
                            !!error
                          }
                          onChange={(event) =>
                            void update(host, { map: event.target.value })
                          }
                        >
                          {maps.map((map) => (
                            <option key={map} value={map}>
                              {mapName(map)}
                            </option>
                          ))}
                        </select>
                      </label>
                      {host.error && (
                        <p className="error" role="alert">
                          {host.error}
                        </p>
                      )}
                      <div className="server-footer">
                        <span className="small muted">
                          {!host.lastSeen
                            ? "Waiting for agent"
                            : !host.online
                              ? "Agent disconnected"
                              : transitioning
                                ? `Requested: ${host.desiredState}`
                                : "Agent connected"}
                        </span>
                        <button
                          className={
                            host.desiredState === "running"
                              ? "secondary"
                              : "primary"
                          }
                          disabled={
                            (!host.actions.start && !host.actions.stop) ||
                            busy === host.id ||
                            !!error
                          }
                          onClick={() =>
                            void update(host, {
                              desiredState:
                                host.desiredState === "running"
                                  ? "stopped"
                                  : "running",
                            })
                          }
                        >
                          {busy === host.id
                            ? "Saving…"
                            : host.actions.stop
                              ? "■ Stop server"
                              : state === "stopping"
                                ? "Stopping…"
                                : "▶ Start server"}
                        </button>
                      </div>
                    </div>
                  </article>
                );
              })}
            </div>
          )}
        </main>
      </div>
      {notice && (
        <div className="toast" role="status">
          ✓ {notice}
        </div>
      )}
      {(adding || credentials) && (
        <Dialog
          onClose={() => {
            if (busy !== "create") {
              setAdding(false);
              setCredentials(null);
            }
          }}
        >
          {credentials ? (
            <>
              <h2 id="modal-title">Connect {credentials.host.name}.</h2>
              <p className="muted">
                Build the agent, then run this on the same machine as the API to
                try simulation. The token is shown once. Keep it private.
              </p>
              <pre>{agentCommand}</pre>
              <button
                className="primary"
                onClick={() => void copy(agentCommand)}
              >
                Copy agent command
              </button>
              <p className="small muted">
                For a remote host, use your HTTPS API address. For real CS2,
                replace --simulate with --unit and --config as described in
                README.md.
              </p>
            </>
          ) : (
            <>
              <h2 id="modal-title">Register a host.</h2>
              <p className="muted">
                Connect an existing Linux host. This registers its server; it
                does not install CS2.
              </p>
              <form onSubmit={(event) => void addHost(event)}>
                <label>
                  Server name
                  <input
                    autoFocus
                    name="name"
                    required
                    maxLength={80}
                    placeholder="Friday night crew"
                  />
                </label>
                <label>
                  Game hostname or IP
                  <input
                    name="address"
                    required
                    maxLength={253}
                    placeholder="play.example.com"
                  />
                </label>
                <label>
                  Game port
                  <input
                    name="port"
                    type="number"
                    min={1}
                    max={65535}
                    defaultValue={27015}
                    required
                  />
                </label>
                {error && (
                  <p className="error" role="alert">
                    {error}
                  </p>
                )}
                <button
                  className="primary"
                  type="submit"
                  disabled={busy === "create"}
                >
                  {busy === "create" ? "Registering…" : "Register host →"}
                </button>
              </form>
            </>
          )}
        </Dialog>
      )}
    </div>
  );
}

function Dialog({
  children,
  onClose,
}: {
  children: ReactNode;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current!;
    dialog.showModal();
    return () => dialog.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className="modal"
      aria-labelledby="modal-title"
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <button
        className="close text-button"
        aria-label="Close dialog"
        onClick={onClose}
      >
        ✕
      </button>
      {children}
    </dialog>
  );
}
