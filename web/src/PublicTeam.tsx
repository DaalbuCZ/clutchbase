import { useEffect, useState } from "react";
import type { Team, TeamMember } from "./api";
import "./PublicTeam.css";

type Confirmation = { kind: "remove" | "transfer" | "disband"; revision: number; member?: TeamMember };
export function PublicTeam({ team, steamId, live, busy, invite, onAction }: { team: Team; steamId: string; live: boolean; busy: boolean; invite: string; onAction: (path: string, options: RequestInit) => Promise<boolean> }) {
  const [copyError, setCopyError] = useState(false);
  async function copyInviteLink() {
    try { await navigator.clipboard.writeText(`${window.location.origin}/play#teams?invite=${encodeURIComponent(invite)}`); setCopyError(false); }
    catch { setCopyError(true); }
  }
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [submitted, setSubmitted] = useState(false);
  const stale = !!confirmation && confirmation.revision !== team.revision;
  const actions = team.actions;
  useEffect(() => { if (submitted && stale) { setConfirmation(null); setSubmitted(false); } }, [submitted, stale]);
  function confirm(kind: Confirmation["kind"], member?: TeamMember) { setConfirmation({ kind, member, revision: team.revision }); setSubmitted(false); }
  const allowed = confirmation?.kind === "remove" ? actions?.removeMember : confirmation?.kind === "transfer" ? actions?.transferCaptain : actions?.disband;
  async function submit() {
    if (!confirmation || stale || !allowed || !live) return;
    const { kind, member, revision } = confirmation;
    const path = kind === "remove" ? `/player/teams/${team.id}/members/${member!.steamId}` : kind === "transfer" ? `/player/teams/${team.id}/captain` : `/player/teams/${team.id}`;
    const success = await onAction(path, { method: kind === "transfer" ? "PATCH" : "DELETE", body: JSON.stringify({ revision, ...(kind === "transfer" ? { steamId: member!.steamId } : {}) }) });
    if (success) setSubmitted(true);
  }
  return <article className="server-card public-team"><div className="server-body">
    <div className="server-card-head"><div><h2>{team.name}</h2><p className="server-meta">{team.members.length}/5 players · {team.captainSteamId === steamId ? "You are captain" : "Team member"}</p></div></div>
    {team.rosterLocked && <p className="team-lock" role="status">The roster is locked while the team is checked in to a tournament.</p>}
    <ul className="team-members">{team.members.map((member) => <li key={member.steamId}>
      <div><strong>{member.gameName || `Player ${member.steamId.slice(-4)}`}</strong><span className="small muted">{member.steamId === team.captainSteamId ? "Captain" : "Member"}{member.steamId === steamId ? " · You" : ""}</span></div>
      {member.steamId !== team.captainSteamId && <div className="heading-actions">
        {actions?.transferCaptain && <button className="secondary" disabled={!live || busy || submitted} onClick={() => confirm("transfer", member)}>Make captain</button>}
        {actions?.removeMember && <button className="text-button" disabled={!live || busy || submitted} onClick={() => confirm("remove", member)}>Remove member</button>}
      </div>}
    </li>)}</ul>
    <div className="server-footer">
      {actions?.invite && <button className="secondary" disabled={!live || busy || submitted} onClick={() => void onAction(`/player/teams/${team.id}/invite`, { method: "POST" })}>{busy ? "Generating…" : "Copy invite code"}</button>}
      {actions?.leave && <button className="secondary" disabled={!live || busy || submitted} onClick={() => void onAction("/player/teams/leave", { method: "POST" })}>Leave team</button>}
      {actions?.disband && <button className="text-button" disabled={!live || busy || submitted} onClick={() => confirm("disband")}>Disband team</button>}
    </div>
    {invite && actions?.invite && <div className="team-invite small muted"><p>Invite code: <code>{invite}</code></p><button className="secondary" onClick={() => void copyInviteLink()}>Copy invite link</button>{copyError && <p role="alert">Clipboard unavailable. Copy this link: <code>{`${window.location.origin}/play#teams?invite=${encodeURIComponent(invite)}`}</code></p>}</div>}
    {team.registered && !team.rosterLocked && <p className="small muted">The team can be disbanded after its tournament registrations finish.</p>}
    {confirmation && <section className="team-confirmation" aria-label="Confirm team change">
      <p>{confirmation.kind === "remove" ? `Remove ${confirmation.member?.gameName || confirmation.member?.steamId} from ${team.name}?` : confirmation.kind === "transfer" ? `Make ${confirmation.member?.gameName || confirmation.member?.steamId} captain of ${team.name}? You will become a regular member.` : `Disband ${team.name}? All members will leave; completed tournament records will be retained.`}</p>
      {stale && <p className="error" role="alert">The team changed. Cancel and choose the action again.</p>}
      {submitted && <p className="small muted" role="status">Request accepted. Waiting for the updated team.</p>}
      <div className="heading-actions"><button className="primary" disabled={!live || busy || stale || submitted || !allowed} onClick={() => void submit()}>{confirmation.kind === "remove" ? "Confirm removal" : confirmation.kind === "transfer" ? "Confirm captain transfer" : "Confirm disband"}</button><button className="secondary" disabled={busy} onClick={() => { setConfirmation(null); setSubmitted(false); }}>Cancel</button></div>
    </section>}
  </div></article>;
}
