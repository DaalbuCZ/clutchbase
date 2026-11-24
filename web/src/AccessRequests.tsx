import { useCallback, useEffect, useRef, useState } from "react";
import { APIError, request } from "./api";
import type { AdminAccessRequest } from "./api";
import { connectLive } from "./live";

const statusLabel: Record<string, string> = { pending: "Pending", approved: "Approved", rejected: "Rejected", revoked: "Revoked" };
const errorMessage = (error: unknown) => error instanceof Error ? error.message : "Something went wrong";

export function AccessRequests({ token }: { token: string }) {
  const [items, setItems] = useState<AdminAccessRequest[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState<{ id: string; action: "approve" | "reject"; revision: number; requiresWhitelistEnable: boolean } | null>(null);
  const [enableWhitelistConfirmed, setEnableWhitelistConfirmed] = useState(false);
  const [error, setError] = useState("");
  const [lastLoaded, setLastLoaded] = useState("");
  const [live, setLive] = useState(false);
  const liveEpoch = useRef(0);

  const refresh = useCallback(async () => {
    setLoading(true); setError("");
    const epoch = liveEpoch.current;
    try {
      const next = await request<AdminAccessRequest[]>(token, "/access-requests");
      if (liveEpoch.current === epoch) { setItems(next); setLastLoaded(new Date().toISOString()); }
    }
    catch (err) { setError(errorMessage(err)); }
    finally { setLoading(false); }
  }, [token]);

  useEffect(() => { void refresh(); }, [refresh]);
  useEffect(() => { setEnableWhitelistConfirmed(false); }, [confirm?.id, confirm?.action, confirm?.revision, confirm?.requiresWhitelistEnable]);
  useEffect(() => connectLive<AdminAccessRequest[]>("access-requests", token, (next) => {
    liveEpoch.current += 1;
    setItems(next); setLastLoaded(new Date().toISOString()); setError("");
  }, setLive, () => setError("Administrator session expired. Sign in again.")), [token]);

  async function mutate(item: AdminAccessRequest, action: "approve" | "reject", enableWhitelist = false) {
    setBusy(item.id); setError("");
    try {
      await request(token, `/access-requests/${item.id}`, { method: "PATCH", body: JSON.stringify({ revision: item.revision, action, ...(enableWhitelist ? { enableWhitelist: true } : {}) }) });
      setConfirm(null); await refresh();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) await refresh();
      setError(errorMessage(err));
    } finally { setBusy(""); }
  }

  return <section className="match-rooms access-requests" aria-labelledby="admin-access-requests-heading">
    <div className="page-heading"><div><h1 id="admin-access-requests-heading">Access requests</h1><p className="small muted">Review player requests before granting server access.</p></div><button className="secondary" type="button" disabled={loading} onClick={() => void refresh()}>Refresh</button></div>
    {error && <p className="error" role="alert">{error}</p>}
    {loading && items.length === 0 ? <p role="status">Loading access requests…</p> : items.length === 0 ? <div className="empty"><p>No access requests.</p></div> : <div className="server-grid">{items.map((item) => <article className="server-card access-request-card" key={item.id}><div className="server-body"><div className="server-card-head"><div><h2>{item.hostName}</h2><p className="server-meta">{item.gameName || "Unnamed player"} · {item.steamId}</p></div><span className={`state-pill state-${item.status}`}>{statusLabel[item.status] || item.status}</span></div><p className="small muted">Revision {item.revision}</p>{item.blocker && <p className="small muted" role="status">{item.blocker}</p>}
      {item.status === "pending" && <div className="heading-actions">{confirm?.id === item.id ? (() => {
        const changed = confirm.revision !== item.revision || confirm.requiresWhitelistEnable !== item.requiresWhitelistEnable;
        const needsWhitelistConfirmation = confirm.action === "approve" && confirm.requiresWhitelistEnable;
        const valid = !changed && (confirm.action === "reject" || item.canApprove) && (!needsWhitelistConfirmation || enableWhitelistConfirmed);
        return <>
          {changed && <span className="small error" role="alert">Request changed. Close this confirmation and review it.</span>}
          {needsWhitelistConfirmation && <div className="access-request-warning">
            <p className="small"><strong>Approval will enable whitelist enforcement.</strong> Other players who use only the password and are not on the roster will lose access.</p>
            <label><input type="checkbox" checked={enableWhitelistConfirmed} disabled={!live || changed || busy === item.id} onChange={(event) => setEnableWhitelistConfirmed(event.target.checked)} /> I understand that approving this request enables the whitelist and removes password-only access for players not on the roster.</label>
          </div>}
          {!changed && confirm.action === "approve" && !item.canApprove && <span className="small error" role="alert">This request cannot be approved in the current server state.</span>}
          <span className="small">Confirm {confirm.action}?</span>
          <button className="primary" disabled={!live || !valid || busy === item.id} onClick={() => void mutate(item, confirm.action, needsWhitelistConfirmation)}>{busy === item.id ? "Saving…" : `Confirm ${confirm.action}`}</button>
          <button className="secondary" disabled={busy === item.id} onClick={() => setConfirm(null)}>Cancel</button>
        </>;
      })() : <><button className="primary" disabled={!live || !item.canApprove || !!busy} onClick={() => setConfirm({ id: item.id, action: "approve", revision: item.revision, requiresWhitelistEnable: item.requiresWhitelistEnable })}>Approve</button><button className="secondary" disabled={!live || !!busy} onClick={() => setConfirm({ id: item.id, action: "reject", revision: item.revision, requiresWhitelistEnable: item.requiresWhitelistEnable })}>Reject</button></>}</div>}
    </div></article>)}</div>}
    <p className="small muted">{live ? "Live updates connected" : "Live updates unavailable; use Refresh"}{lastLoaded && ` · Last loaded ${new Date(lastLoaded).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`}</p>
  </section>;
}
