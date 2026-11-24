import "./ReporterInventory.css";

export type ReporterInventoryData = {
  status: "unknown" | "unavailable" | "current" | "different";
  installedHash: string;
  stagedHash: string;
  checkedAt: string | null;
};

export function ReporterInventory({ inventory }: { inventory: ReporterInventoryData }) {
  const status = {
    unknown: "Unknown",
    unavailable: "Installed bundle unavailable or invalid",
    current: "Matches staged bundle",
    different: "Differs from staged bundle",
  }[inventory.status];
  const fingerprint = (name: string, value: string) => <span className="reporter-fingerprint"><strong>{name}:</strong> {value ? <code title={value} aria-label={`${name}: ${value}`}>{value.slice(0, 12)}…</code> : "unavailable"}</span>;
  return <div className="maintenance-item">
    <div>
      <strong>Clutchbase Access file inventory</strong>
      <p className="small muted">{status}. File fingerprints do not confirm the plugin loaded in CS2.</p>
      <p className="small muted">{fingerprint("Installed SHA-256", inventory.installedHash)}<br />{fingerprint("Staged SHA-256", inventory.stagedHash)}</p>
      {inventory.checkedAt && <p className="small muted">Last reported: {new Date(inventory.checkedAt).toLocaleString()}</p>}
    </div>
  </div>;
}
