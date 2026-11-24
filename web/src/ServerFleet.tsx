import type { Host, Machine } from "./api";
import { connection } from "./api";
import "./ServerFleet.css";

type Props = { hosts: Host[]; machines: Machine[]; onSelect: (id: string) => void };

const statusNames: Record<string, string> = {
  uninstalled: "Needs install", installing: "Installing", starting: "Starting",
  running: "Online", stopping: "Stopping", stopped: "Stopped", offline: "Agent offline",
  error: "Error", removing: "Removing",
};

function currentMap(host: Host): string {
  const observing = host.online && host.desiredState === "running" &&
    (host.observedState === "running" || host.observedState === "starting");
  const map = observing
    ? (host.canObserveMap || host.driver === "simulated" ? host.activeMap : "")
    : host.map;
  return map ? map.replace(/^de_/, "").replace(/^./, (letter) => letter.toUpperCase()) : "Current map unavailable";
}

export function ServerFleet({ hosts, machines, onSelect }: Props) {
  const machineNames = new Map(machines.map((machine) => [machine.id, machine.name]));
  return (
    <div className="server-fleet">
      {hosts.map((host) => {
        const warnings = [
          host.error ? "Server error" : "",
          host.gameBuildStatus === "outdated" ? "CS2 update available" : "",
          host.accessStatus === "blocked" ? "Access verification blocked" : "",
        ].filter(Boolean);
        return (
          <button className="server-fleet-row" id={`view-server-${host.id}`} aria-label={`Manage ${host.name}`} key={host.id} onClick={() => onSelect(host.id)}>
            <span className="server-fleet-main">
              <span className="server-fleet-title">{host.name}{host.driver === "simulated" && <span className="server-fleet-simulation">Simulation</span>}</span>
              <span className="server-fleet-details">
                <span>{currentMap(host)}</span><span className="server-fleet-address">{connection(host)}</span>
                {host.status === "running" && host.players != null && host.maxPlayers != null && <span>{host.players}/{host.maxPlayers} players</span>}
                {machineNames.has(host.machineId) && <span>{machineNames.get(host.machineId)}</span>}
              </span>
              {warnings.length > 0 && <span className="server-fleet-warning">{warnings[0]}</span>}
            </span>
            <span className={`server-fleet-status server-fleet-${host.status}`}><i aria-hidden="true" />{statusNames[host.status] || host.status}</span>
          </button>
        );
      })}
    </div>
  );
}
