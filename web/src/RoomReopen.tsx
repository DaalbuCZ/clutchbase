import { useEffect, useState } from "react";
import { request } from "./api";

export function RoomReopen({ id, title, revision, allowed, connected, token }: { id: string; title: string; revision?: number; allowed: boolean; connected: boolean; token?: string }) {
  const [confirmation, setConfirmation] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (submitted && (!allowed || revision !== confirmation)) { setConfirmation(null); setSubmitted(false); }
  }, [submitted, allowed, revision, confirmation]);
  const stale = confirmation !== null && (!allowed || confirmation !== revision);
  if (!allowed && confirmation === null) return null;
  async function reopen() {
    if (confirmation === null || stale || !connected) return;
    setBusy(true); setError("");
    try {
      await request(token || "", `${token ? "/matches" : "/player/matches"}/${id}/reopen`, { method: "POST", body: JSON.stringify({ settingsRevision: confirmation }) });
      setSubmitted(true);
    } catch (err) { setError(err instanceof Error ? err.message : "Could not reopen room"); }
    finally { setBusy(false); }
  }
  return <div className="team-confirmation" aria-label={`Reopen ${title}`}>
    {confirmation === null ? <button className="secondary" disabled={!connected || busy || revision == null} onClick={() => { setConfirmation(revision!); setError(""); }}>Reopen room</button> : <>
      <p>{submitted ? "Reopen requested. Waiting for the room to update." : "Reopen this room? Everyone will need to ready up again."}</p>
      {stale && <p className="error" role="alert">The room or server changed. Cancel and review the current state.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      <div className="heading-actions"><button className="secondary" disabled={!connected || busy || stale || submitted} onClick={() => void reopen()}>{busy ? "Reopening…" : "Confirm reopen"}</button><button className="secondary" disabled={busy} onClick={() => { setConfirmation(null); setSubmitted(false); setError(""); }}>Cancel</button></div>
    </>}
  </div>;
}
