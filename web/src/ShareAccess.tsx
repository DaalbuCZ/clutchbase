import { useState } from "react";
import { viewHash } from "./view";

export function ShareAccess({ id, title }: { id: string; title: string }) {
  const [message, setMessage] = useState("");
  const [fallback, setFallback] = useState(false);
  const url = `${window.location.origin}/play${viewHash("access", id)}`;
  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setFallback(false); setMessage("Access link copied. Players still need your approval.");
    } catch {
      setFallback(true); setMessage("Copy the access link below. Players still need your approval.");
    }
  }
  return <div className="share-room">
    <button type="button" className="secondary" aria-label={`Share access for ${title}`} onClick={() => void copy()}>Copy access link</button>
    {message && <p className="small muted" role="status">{message}</p>}
    {fallback && <label>Access link<input readOnly value={url} onFocus={(event) => event.currentTarget.select()} /></label>}
  </div>;
}
