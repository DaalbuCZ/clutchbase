import { useEffect, useRef, useState } from "react";
import type { Tournament } from "./api";
import "./TournamentPicker.css";

const statusLabels: Record<string, string> = { open: "Registration open", checkin: "Team check-in", active: "Active", complete: "Completed", cancelled: "Cancelled" };
export function TournamentPicker({ tournaments, selectedID, loading, onSelect, onBack }: { tournaments: Tournament[]; selectedID: string; loading: boolean; onSelect: (id: string) => void; onBack: () => void }) {
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("current");
  const previous = useRef("");
  const search = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (selectedID) previous.current = selectedID;
    else if (previous.current) {
      const row = document.getElementById(`view-tournament-${previous.current}`);
      if (row) row.focus(); else search.current?.focus();
    }
  }, [selectedID]);
  if (loading) return <p role="status">Loading tournaments…</p>;
  if (selectedID) return <div className="tournament-detail-nav"><button className="secondary" onClick={onBack}>← Back to tournaments</button>{!tournaments.some((t) => t.id === selectedID) && <p role="status">This tournament is not available in the current snapshot.</p>}</div>;
  const needle = query.trim().toLocaleLowerCase();
  const shown = tournaments.filter((t) => (status === "all" || (status === "current" ? ["open", "checkin", "active"].includes(t.status) : t.status === status)) && (!needle || [t.name, ...t.entries.map((e) => e.teamName)].some((name) => name.toLocaleLowerCase().includes(needle))));
  return <section aria-label="Find tournament" className="tournament-picker">
    <div className="tournament-search"><label>Find tournament<input ref={search} type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Tournament or team name" /></label><label>Status<select value={status} onChange={(event) => setStatus(event.target.value)}><option value="current">Current tournaments</option><option value="open">Registration open</option><option value="checkin">Team check-in</option><option value="active">Active</option><option value="complete">Completed</option><option value="cancelled">Cancelled</option><option value="all">All tournaments</option></select></label><span className="small muted" role="status">{shown.length} of {tournaments.length} tournaments</span></div>
    {shown.length === 0 ? <div className="empty"><p>{tournaments.length === 0 ? "No tournaments yet." : "No tournaments match these filters."}</p>{tournaments.length > 0 && <button className="secondary" onClick={() => { setStatus("all"); setQuery(""); }}>Show all tournaments</button>}</div> : <div className="tournament-list">{shown.map((t) => <button className="tournament-list-item" key={t.id} id={`view-tournament-${t.id}`} aria-label={`View ${t.name}`} onClick={() => onSelect(t.id)}>
      <span className="tournament-list-main"><strong>{t.name}</strong><small>{t.format === "round_robin" ? "Round robin" : t.format === "double_elimination" ? "Double elimination" : "Single elimination"} · BO{t.bestOf} · {t.playersPerSide}v{t.playersPerSide} · {t.entries.length} teams{t.myEntryStatus ? ` · Your team: ${t.myEntryStatus}` : ""}</small></span>
      <span className={`state-pill ${t.status === "active" ? "state-running" : "state-stopped"}`}>{statusLabels[t.status] || t.status}</span><span aria-hidden="true">→</span>
    </button>)}</div>}
  </section>;
}
