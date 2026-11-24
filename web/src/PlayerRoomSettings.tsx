import { useState } from "react";
import type { FormEvent } from "react";
import { request } from "./api";
import type { PlayerMatch } from "./api";

type Draft = { revision: number; title: string; map: string; size: number; overtime: boolean; manualReady: boolean };
export function PlayerRoomSettings({ match, maps, connected }: { match: PlayerMatch; maps: string[]; connected: boolean }) {
  const [draft, setDraft] = useState<Draft | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const stale = !!draft && (!match.actions.edit || match.settingsRevision !== draft.revision);
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft || stale || !connected) return;
    const data = new FormData(event.currentTarget);
    setBusy(true); setError(""); setNotice("");
    try {
      const result = await request<{ readinessReset: boolean }>("", `/player/matches/${match.id}/settings`, { method: "PATCH", body: JSON.stringify({ settingsRevision: draft.revision, title: data.get("title"), map: data.get("map"), playersPerSide: Number(data.get("playersPerSide")), overtime: data.get("overtime") !== null, manualReady: data.get("manualReady") !== null }) });
      setDraft(null); setNotice(result.readinessReset ? "Settings saved. Everyone must ready up again." : "Settings saved.");
    } catch (err) { setError(err instanceof Error ? err.message : "Could not save room settings"); }
    finally { setBusy(false); }
  }
  if (!match.actions.edit && !draft && !notice) return null;
  return <section aria-label={`Room settings for ${match.title}`}>
    {!draft ? <>{match.actions.edit && <button className="secondary" disabled={!connected || busy || !match.settingsRevision} onClick={() => { setDraft({ revision: match.settingsRevision!, title: match.title, map: match.map, size: match.playersPerSide, overtime: match.overtime, manualReady: !!match.manualReady }); setError(""); setNotice(""); }}>Edit room</button>}{notice && <p className="small muted" role="status">{notice}</p>}</> : <form className="match-create-form" onSubmit={(event) => void save(event)}>
      <label>Match name<input name="title" required maxLength={80} defaultValue={draft.title} /></label>
      <label>Map<select name="map" defaultValue={draft.map} required>{maps.map((map) => <option key={map} value={map}>{map.replace(/^de_/, "")}</option>)}</select></label>
      <label>Room size<select name="playersPerSide" defaultValue={draft.size}>{[1,2,3,4,5].map((size) => <option key={size} value={size}>{size}v{size}</option>)}</select></label>
      <label className="overtime-choice"><input type="checkbox" name="overtime" defaultChecked={draft.overtime} /> Enable overtime</label>
      <label className="overtime-choice"><input type="checkbox" name="manualReady" defaultChecked={draft.manualReady} /> Require in-game ready</label>
      <p className="small muted">Players must use .ready in CS2 before the match starts. This option requires an updated host agent and reporter. Uneven teams always use in-game ready.</p>
      <p className="small muted">Changing the map, room size, overtime or in-game ready setting clears ready checks. Both sides must fit in the chosen size.</p>
      {stale && <p className="error" role="alert">The room changed. Cancel and reopen the editor.</p>}
      {error && <p className="error" role="alert">{error}</p>}
      <div className="heading-actions"><button className="primary" disabled={!connected || busy || stale}>{busy ? "Saving…" : "Save room settings"}</button><button type="button" className="secondary" disabled={busy} onClick={() => { setDraft(null); setError(""); }}>Cancel</button></div>
    </form>}
  </section>;
}
