import { useEffect, useState } from "react";

type FixtureSchedule = { id: string; scheduledAt?: string | null; scheduleRevision?: number; canSchedule?: boolean };

function localInput(value?: string | null) {
  if (!value) return "";
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "";
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

export function TournamentSchedule({ match, admin = false, connected, busy = false, onSave }: { match: FixtureSchedule; admin?: boolean; connected: boolean; busy?: boolean; onSave?: (revision: number, scheduledAt: string | null) => Promise<boolean> }) {
  const [edit, setEdit] = useState<{ revision: number; value: string } | null>(null);
  const [pendingRevision, setPendingRevision] = useState<number | null>(null);
  const [error, setError] = useState("");
  useEffect(() => { if (pendingRevision !== null && match.scheduleRevision !== pendingRevision) { setPendingRevision(null); setEdit(null); } }, [pendingRevision, match.scheduleRevision]);
  const stale = !!edit && (edit.revision !== match.scheduleRevision || !match.canSchedule);
  const date = match.scheduledAt ? new Date(match.scheduledAt) : null;
  const validDate = date && Number.isFinite(date.getTime());
  async function save(clear: boolean) {
    if (!edit || stale || !onSave) return;
    const parsed = clear ? null : new Date(edit.value);
    if (!clear && (!edit.value || !parsed || !Number.isFinite(parsed.getTime()))) { setError("Choose a valid date and time."); return; }
    setError("");
    setPendingRevision(edit.revision);
    if (!await onSave(edit.revision, parsed ? parsed.toISOString() : null)) setPendingRevision(null);
  }
  return <div className="fixture-schedule">
    {validDate ? <p className="small muted">Scheduled: <time dateTime={match.scheduledAt!}>{date.toLocaleString(undefined, { year: "numeric", month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZoneName: "short" })}</time></p> : <p className="small muted">Not scheduled</p>}
    {admin && match.canSchedule && !edit && <button type="button" className="secondary" disabled={!connected || busy} onClick={() => setEdit({ revision: match.scheduleRevision ?? 0, value: localInput(match.scheduledAt) })}>Schedule fixture</button>}
    {admin && edit && <div className="team-confirmation">
      <label>Local date and time<input type="datetime-local" value={edit.value} onChange={(event) => setEdit({ ...edit, value: event.target.value })} disabled={pendingRevision !== null} /></label>
      {stale && <p className="error" role="alert">The fixture changed. Cancel and review the latest schedule.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      {pendingRevision !== null && <p className="small muted" role="status">Waiting for the updated schedule…</p>}
      <div className="heading-actions"><button type="button" className="primary" disabled={!connected || busy || stale || pendingRevision !== null} onClick={() => void save(false)}>Save time</button>{match.scheduledAt && <button type="button" className="secondary" disabled={!connected || busy || stale || pendingRevision !== null} onClick={() => void save(true)}>Clear time</button>}<button type="button" className="secondary" disabled={busy || pendingRevision !== null} onClick={() => setEdit(null)}>Cancel</button></div>
    </div>}
  </div>;
}
