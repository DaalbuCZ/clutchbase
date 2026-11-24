import type { Machine } from "./api";
import "./MachineResources.css";

export function MachineResources({ machine, live, compact = false }: { machine: Machine; live: boolean; compact?: boolean }) {
  if (machine.driver !== "managed") return null;
  if (!machine.online) return <p className="small muted machine-resources-unavailable">Machine resources unavailable · agent disconnected</p>;
  const resources = machine.resources;
  if (!resources) return <p className="small muted machine-resources-unavailable">CPU and memory unavailable</p>;
  const used = resources.memoryTotalBytes - resources.memoryAvailableBytes;
  const memoryPercent = used / resources.memoryTotalBytes * 100;
  return <section className={`machine-resources ${compact ? "machine-resources-compact" : ""}`} aria-label={`Resources on ${machine.name}`}>
    {!live && <p className="small muted" role="status">Last received measurements · reconnecting</p>}
    <div className="machine-resource"><span>CPU <strong>{resources.cpuPercent === null ? "Awaiting sample" : `${resources.cpuPercent.toFixed(1)}%`}</strong></span>{resources.cpuPercent !== null && <meter min={0} max={100} value={resources.cpuPercent} aria-label={`CPU use on ${machine.name}`} />}</div>
    <div className="machine-resource"><span>Memory <strong>{(used / 1024 ** 3).toFixed(1)} / {(resources.memoryTotalBytes / 1024 ** 3).toFixed(1)} GiB</strong></span><meter min={0} max={100} value={memoryPercent} aria-label={`Memory use on ${machine.name}`} /></div>
    {!compact && <p className="small muted">Machine-wide usage. CPU is averaged across cores since the previous sample.</p>}
  </section>;
}
