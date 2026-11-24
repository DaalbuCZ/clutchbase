import { useEffect, useState } from "react";

export type FixtureDispute = { reason: string; resolution?: string; revision: number };
export function TournamentDispute({ dispute, canDispute = false, admin = false, connected, busy = false, onSubmit, onResolve }: {
  dispute?: FixtureDispute | null; canDispute?: boolean; admin?: boolean; connected: boolean; busy?: boolean;
  onSubmit?: (reason: string) => Promise<boolean>; onResolve?: (revision: number, resolution: string) => Promise<boolean>;
}) {
  const [text, setText] = useState("");
  const [revision, setRevision] = useState<number | null>(null);
  const [pending, setPending] = useState(false);
  useEffect(() => {
    if (pending) { setText(""); setRevision(null); setPending(false); }
  }, [dispute?.revision, dispute?.reason]);
  const stale = admin && revision !== null && revision !== dispute?.revision;
  const tooLong = new TextEncoder().encode(text.trim()).length > 2000;
  async function submit() {
    if (!text.trim() || stale || tooLong) return;
    setPending(true);
    const ok = admin && dispute ? await onResolve?.(revision ?? dispute.revision, text.trim()) : await onSubmit?.(text.trim());
    if (!ok) setPending(false);
  }
  if (!dispute && !canDispute) return null;
  return <section aria-label="Fixture dispute">
    {dispute && <><p><strong>Dispute:</strong> {dispute.reason}</p><p role="status">{dispute.resolution ? `Resolved: ${dispute.resolution}` : "Awaiting administrator review"}</p></>}
    {((canDispute && !dispute) || (admin && dispute && !dispute.resolution)) && <>
      <label>{admin ? "Review decision" : "Reason for dispute"}<textarea maxLength={2000} value={text} disabled={!connected || busy || pending} onChange={(event) => { setText(event.target.value); if (admin && revision === null) setRevision(dispute?.revision ?? null); }} /></label>
      {stale && <p role="alert">The dispute changed. <button type="button" className="text-button" onClick={() => { setText(""); setRevision(null); }}>Discard draft and review the latest version</button></p>}
      {tooLong && <p role="alert">The text is too long. Shorten it before submitting.</p>}
      {admin && <p className="small muted">The review records a decision; it does not change the result or bracket.</p>}
      <button type="button" disabled={!connected || busy || pending || stale || tooLong || !text.trim()} onClick={() => void submit()}>{admin ? "Resolve dispute" : "Submit dispute"}</button>
    </>}
  </section>;
}
