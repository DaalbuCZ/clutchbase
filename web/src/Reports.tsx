import { useEffect, useState } from "react";
import { connectLive } from "./live";
import { request } from "./api";

type Report = { id: string; matchId: string; matchTitle: string; hostName: string; reporterSteamId?: string; reporterName?: string; description: string; status: "open" | "resolved"; revision: number; createdAt: string };
type ReportsSnapshot = { capturedAt: string; reports: Report[]; truncated: boolean };

export function Reports({ token, player = false }: { token?: string; player?: boolean }) {
  const [snapshot, setSnapshot] = useState<ReportsSnapshot | null>(null);
  const [online, setOnline] = useState(false);
  const [expired, setExpired] = useState(false);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  useEffect(() => connectLive<ReportsSnapshot>(player ? "player-reports" : "reports", token,
    (next) => setSnapshot((old) => !old || next.capturedAt >= old.capturedAt ? next : old), setOnline, () => setExpired(true)), [player, token]);
  async function review(report: Report) {
    setBusy(report.id); setError("");
    try { await request(token || "", `/reports/${report.id}`, { method: "PATCH", body: JSON.stringify({ status: report.status === "open" ? "resolved" : "open", revision: report.revision }) }); }
    catch (err) { setError(err instanceof Error ? err.message : "Could not update report"); }
    finally { setBusy(""); }
  }
  return <section className="match-rooms" aria-label={player ? "My reports" : "Reports"}>
    {!player && <div className="page-heading"><h1>Reports</h1></div>}
    {expired ? <p className="error" role="alert">Session expired. Sign in again.</p> : !snapshot ? <p role="status">Connecting to reports…</p> : <>
      {!online && <p role="status">Reconnecting… Showing the last received reports.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      {snapshot.reports.length === 0 && <p className="muted">No reports yet.</p>}
      <div className="server-grid">{snapshot.reports.map((report) => <article className="server-card" key={report.id}><div className="server-body">
        <div className="server-card-head"><div><h2>{report.matchTitle}</h2><p className="server-meta">{report.hostName} · {new Date(report.createdAt).toLocaleString()}{!player && ` · ${report.reporterName} (${report.reporterSteamId})`}</p></div><span className="state-pill">{report.status === "open" ? "Open" : "Resolved"}</span></div>
        <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{report.description}</p>
        {!player && <button className="secondary" disabled={!online || !!busy} onClick={() => void review(report)}>{busy === report.id ? "Saving…" : report.status === "open" ? "Mark resolved" : "Reopen"}</button>}
      </div></article>)}</div>
      {snapshot.truncated && <p className="small muted">Showing the first 100 reports, with open reports first.</p>}
      <p className="small muted">Reports are visible to their author and administrators. Resolving a report does not change the match or penalize a player.</p>
    </>}
  </section>;
}
