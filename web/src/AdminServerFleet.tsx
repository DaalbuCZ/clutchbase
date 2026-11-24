import type { Host, Machine } from "./api";
import { connection } from "./api";
import "./ServerFleet.css";
import "./AdminOperations.css";

const statusNames: Record<string, string> = {
  uninstalled: "Needs install", installing: "Installing", starting: "Starting",
  running: "Online", stopping: "Stopping", stopped: "Stopped", offline: "Agent offline",
  error: "Error", removing: "Removing",
};

export function AdminServerFleet({ hosts, machines, live, onSelect }: { hosts: Host[]; machines: Machine[]; live: boolean; onSelect: (id: string) => void }) {
  return <div className="admin-fleet">{hosts.map((host) => {
    const machine = machines.find((item) => item.id === host.machineId);
    const observing = host.online && host.desiredState === "running" && (host.observedState === "running" || host.observedState === "starting");
    const map = observing ? (host.canObserveMap || host.driver === "simulated" ? host.activeMap : "") : host.map;
    const attention = [
      !host.online ? "Agent disconnected" : "",
      host.error || host.status === "error" ? "Server error" : "",
      host.gameUpdateStatus === "failed" ? "CS2 update failed" : host.gameBuildStatus === "outdated" ? "CS2 update available" : "",
      host.reporterUpdateStatus === "failed" ? "Plugin update failed" : host.reporterUpdateAvailable ? "Plugin update available" : "",
      host.restartPhase === "failed" ? "Restart failed" : "",
      host.accessStatus === "blocked" ? "Access verification blocked" : "",
      host.preset != null && !host.presetApplied ? "Preset awaiting agent" : "",
      host.whitelist != null && !host.whitelistApplied ? "Roster awaiting agent" : "",
    ].filter(Boolean);
    const resources = machine?.online ? machine.resources : null;
    const maintenance = [
      host.gameUpdateStatus === "pending" ? "CS2 update pending" : "",
      host.reporterUpdateStatus === "pending" ? "Plugin update pending" : "",
      host.restartPhase === "stopping" || host.restartPhase === "starting" || host.restartPhase === "cancelling" ? `Restart ${host.restartPhase}` : "",
    ].filter(Boolean);
    return <button className="admin-fleet-card server-fleet-row" id={`view-server-${host.id}`} aria-label={`Manage ${host.name}`} key={host.id} onClick={() => onSelect(host.id)}>
      <span className="admin-fleet-head"><strong>{host.name}</strong><span className={`state-pill state-${host.status}`}><i className="tiny-dot" />{statusNames[host.status] || host.status}</span></span>
      <span className="admin-fleet-meta">{machine?.name || "Server"}{host.driver === "simulated" ? " · Simulation" : ""} · {connection(host)}</span>
      <span className="admin-fleet-facts"><span>{map ? map.replace(/^de_/, "").replace(/^./, (letter) => letter.toUpperCase()) : "Current map unavailable"}</span><span>{host.status === "running" && host.players != null && host.maxPlayers != null ? `${host.players}/${host.maxPlayers} players` : `Requested: ${host.desiredState}`}</span></span>
      {attention.length > 0 && <span className="admin-fleet-attention">{attention.join(" · ")}</span>}
      {maintenance.length > 0 && <span className="admin-fleet-maintenance">{maintenance.join(" · ")}</span>}
      {machine && <span className="admin-fleet-resources">{resources ? `${live ? "" : "Last received · "}Machine CPU ${resources.cpuPercent == null ? "awaiting sample" : `${resources.cpuPercent.toFixed(1)}%`} · Memory ${((resources.memoryTotalBytes - resources.memoryAvailableBytes) / 1024 ** 3).toFixed(1)} / ${(resources.memoryTotalBytes / 1024 ** 3).toFixed(1)} GiB` : "Machine resources unavailable"}{machine.online && machine.storage ? ` · ${(machine.storage.availableBytes / 1024 ** 3).toFixed(1)} GiB free` : ""}</span>}
      <span className="admin-fleet-open">Manage server →</span>
    </button>;
  })}</div>;
}
