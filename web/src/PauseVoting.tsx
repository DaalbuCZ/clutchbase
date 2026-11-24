import { useEffect, useState } from "react";

export function PauseVoting({ threshold, votes, voted = false, canVote = false, canConfigure = false, connected, busy, onVote, onConfigure }: {
  threshold: number; votes: number; voted?: boolean; canVote?: boolean; canConfigure?: boolean;
  connected: boolean; busy: boolean; onVote?: () => void; onConfigure?: (threshold: number) => void;
}) {
  const [draft, setDraft] = useState(String(threshold));
  useEffect(() => setDraft(String(threshold)), [threshold]);
  return <section aria-label="Pause voting">
    <p role="status">Pause votes: {votes} / {threshold}{voted ? " · You voted" : ""}</p>
    {canVote && <button type="button" className="secondary" disabled={!connected || busy} onClick={onVote}>Vote to pause</button>}
    {canConfigure && <form onSubmit={(event) => { event.preventDefault(); onConfigure?.(Number(draft)); }}>
      <label>Votes needed to pause <input type="number" min="1" max="10" required value={draft} onChange={(event) => setDraft(event.target.value)} /></label>
      <button type="submit" className="secondary" disabled={!connected || busy}>Save threshold</button>
    </form>}
  </section>;
}
