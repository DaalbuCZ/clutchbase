import { useState } from "react";
import { viewHash } from "./view";

export function ShareRoom({ id, title }: { id: string; title: string }) {
  const [message, setMessage] = useState("");
  const [fallback, setFallback] = useState(false);
  const url = `${window.location.origin}/play${viewHash("matches", id)}`;
  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setFallback(false); setMessage("Match link copied. Friends must have access to this server.");
    } catch {
      setFallback(true); setMessage("Copy the match link below. Friends must have access to this server.");
    }
  }
  return <div className="share-room">
    <button type="button" className="secondary" aria-label={`Share ${title}`} onClick={() => void copy()}>Copy match link</button>
    {message && <p className="small muted" role="status">{message}</p>}
    {fallback && <label>Match link<input readOnly value={url} onFocus={(event) => event.currentTarget.select()} /></label>}
  </div>;
}
