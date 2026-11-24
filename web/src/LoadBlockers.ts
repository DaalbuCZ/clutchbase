const labels: Record<string, string> = {
  workshop_unsupported: "Workshop match loading is not supported by the current reporter.",
  server_offline: "The server must be online and running.",
  server_access_unverified: "Wait for server access verification.",
  reporter_unsupported: "The reporter or agent is not ready for this match setup.",
  server_not_ready: "The server is not ready to load a match. Check its status and current match.",
  players_not_ready: "Both sides need players, and everyone must be ready.",
  player_access_unavailable: "Wait for server access to be verified before loading.",
  load_permission_required: "The room creator or an administrator must load this match.",
};
export function loadBlockerLabels(codes: string[]): string[] {
  return codes.map((code) => labels[code] || "The server reports that this match cannot be loaded yet.");
}
