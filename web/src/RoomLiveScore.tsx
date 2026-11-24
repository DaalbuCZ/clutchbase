import type { LiveScore } from "./api";
import "./RoomLiveScore.css";

export function RoomLiveScore({ score, connected, active }: { score?: LiveScore | null; connected: boolean; active?: boolean }) {
  if (!score) return active ? <p className="small muted" role="status">Current map score unavailable</p> : null;
  return <section className="room-live-score" aria-label="Current map score">
    <div className="room-score-meta"><strong>{score.map.replace(/^de_/, "")}</strong><span>{connected ? "Current map" : "Last received score · reconnecting"}</span></div>
    <p className="room-score-value"><span className="room-score-ct">CT <b>{score.ct}</b></span><span aria-hidden="true">–</span><span className="room-score-t"><b>{score.t}</b> T</span></p>
    <p className="small muted">Current in-game sides · observed <time dateTime={new Date(score.observedAt).toISOString()}>{new Date(score.observedAt).toLocaleTimeString()}</time>. Room lineups show starting assignments.</p>
  </section>;
}
