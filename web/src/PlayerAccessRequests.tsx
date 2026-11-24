import { useEffect, useRef, useState } from "react";
import { connectLive } from "./live";
import { request } from "./api";
import type { PlayerAccessRequest } from "./api";
import { viewHash } from "./view";

type Identity = { steamId: string | null; steamEnabled: boolean; gameName: string };

const statusLabel: Record<string, string> = { pending: "Pending", approved: "Approved", rejected: "Rejected", revoked: "Revoked" };

export function PlayerAccessRequests({ identity, hostId }: { identity: Identity | null; hostId: string }) {
  const [requests, setRequests] = useState<PlayerAccessRequest[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [lastLoaded, setLastLoaded] = useState("");
  const [live, setLive] = useState(false);
  const liveEpoch = useRef(0);

  useEffect(() => {
    if (!identity?.steamId) return;
    let cancelled = false;
    setLoading(true); setError("");
    const epoch = liveEpoch.current;
    void request<PlayerAccessRequest[]>("", "/player/access-requests")
      .then((value) => { if (!cancelled && liveEpoch.current === epoch) { setRequests(value); setLastLoaded(new Date().toISOString()); } })
      .catch((err) => { if (!cancelled) setError(err instanceof Error ? err.message : "Could not load access requests"); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [identity?.steamId]);

  useEffect(() => {
    if (!identity?.steamId) return;
    return connectLive<PlayerAccessRequest[]>("player-access-requests", undefined, (next) => {
      liveEpoch.current += 1;
      setRequests(next); setLastLoaded(new Date().toISOString()); setError("");
    }, setLive, () => setError("Your session expired. Sign in with Steam again."));
  }, [identity?.steamId]);

  async function refresh() {
    setLoading(true); setError("");
    const epoch = liveEpoch.current;
    try {
      const next = await request<PlayerAccessRequest[]>("", "/player/access-requests");
      if (liveEpoch.current === epoch) { setRequests(next); setLastLoaded(new Date().toISOString()); }
    }
    catch (err) { setError(err instanceof Error ? err.message : "Could not load access requests"); }
    finally { setLoading(false); }
  }

  const current = requests.find((item) => item.hostId === hostId);
  async function submit() {
    if (!hostId) return;
    setBusy(true); setError(""); setNotice("");
    try {
      await request("", "/player/access-requests", { method: "POST", body: JSON.stringify({ hostId }) });
      setNotice("Access request submitted. The host must approve it before the server appears in your list.");
      await refresh();
    } catch (err) { setError(err instanceof Error ? err.message : "Could not submit access request"); }
    finally { setBusy(false); }
  }

  if (!hostId && !identity?.steamId) return <section className="match-rooms"><div className="empty"><p>Open a server access link to request access.</p></div></section>;
  if (!identity) return <section className="match-rooms"><div className="empty" role="status">Loading your session…</div></section>;
  if (!identity.steamId) return <section className="match-rooms"><div className="empty">
    <p>Sign in with Steam to request access to this server.</p>
    {identity.steamEnabled ? <a className="primary steam-sign-in" href={`/api/auth/steam?returnTo=${encodeURIComponent(viewHash("access", hostId))}`}>Sign in with Steam ↗</a> : <p className="muted">Steam sign-in is not configured on this instance.</p>}
  </div></section>;

  return <section className="match-rooms access-requests" aria-labelledby="access-requests-heading">
    <div className="page-heading"><div><h2 id="access-requests-heading">Request server access</h2><p className="small muted">Your Steam identity is shared with the server administrator only after you submit this request.</p></div><button className="secondary" type="button" onClick={() => void refresh()} disabled={loading}>Refresh</button></div>
    {error && <p className="error" role="alert">{error}</p>}
    {notice && <p className="small" role="status">{notice}</p>}
    {!hostId && <p className="small muted">Open a server access link to request access to another server.</p>}
    {current ? <article className="server-card access-request-card"><div className="server-body"><div className="server-card-head"><div><h3>{current.hostName}</h3><p className="server-meta">Access request</p></div><span className={`state-pill state-${current.status}`}>{statusLabel[current.status] || current.status}</span></div>
      {current.status === "pending" && <p role="status">Waiting for the host to review your request.</p>}
      {current.status === "approved" && <p role="status">Approved. The server will appear in Servers when its live access snapshot includes you.</p>}
      {current.status === "rejected" && <p role="status">The host rejected this request. Contact the host if your access needs have changed.</p>}
      {current.status === "revoked" && <><p role="status">Your previous approval was revoked. You can submit a new request.</p><button className="primary" type="button" onClick={() => void submit()} disabled={busy}>{busy ? "Submitting…" : "Request access again"}</button></>}
    </div></article> : hostId ? <div className="empty"><p>Request access to this server from its host.</p><button className="primary" type="button" onClick={() => void submit()} disabled={busy}>{busy ? "Submitting…" : "Submit access request"}</button></div> : null}
    {requests.length > 0 && <section className="access-request-history" aria-label="Your access requests"><h3>Your access requests</h3>{requests.map((item) => <div className="machine-row" key={item.id}><strong>{item.hostName}</strong><span className={`state-pill state-${item.status}`}>{statusLabel[item.status] || item.status}</span></div>)}</section>}
    <p className="small muted">{live ? "Live updates connected" : "Live updates unavailable; use Refresh"}{lastLoaded && ` · Last loaded ${new Date(lastLoaded).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`}</p>
  </section>;
}
