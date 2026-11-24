// Reconnect the transport; game lifecycle and permissions always come from snapshots.
export function connectLive<T>(
  view: "manage" | "player" | "history" | "player-history" | "stats" | "player-stats" | "leaderboard" | "player-leaderboard" | "reports" | "player-reports" | "access-requests" | "player-access-requests",
  token: string | undefined,
  snapshot: (value: T) => void,
  connection: (online: boolean) => void,
  unauthorized: () => void,
  before?: string,
  search?: string,
): () => void {
  let stopped = false;
  let socket: WebSocket | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let watchdog: ReturnType<typeof setTimeout> | undefined;
  let delay = 500;
  const alive = () => {
    clearTimeout(watchdog);
    watchdog = setTimeout(() => {
      connection(false);
      socket?.close();
    }, 40000);
  };
  const start = () => {
    if (stopped) return;
    const url = new URL(`/api/live?view=${view}`, window.location.href);
    if (before) url.searchParams.set("before", before);
    if (search) url.searchParams.set("search", search);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    socket = new WebSocket(url);
    socket.onopen = () => {
      if (stopped) return;
      socket?.send(JSON.stringify({ token }));
      alive();
    };
    socket.onmessage = (event: MessageEvent<string>) => {
      if (stopped) return;
      try {
        const value = JSON.parse(event.data) as T & { code?: number; type?: string };
        alive();
        if (value.code === 401) {
          stopped = true;
          clearTimeout(watchdog);
          connection(false);
          socket?.close();
          unauthorized();
        } else if (value.type !== "heartbeat") {
          const arrayView = view === "access-requests" || view === "player-access-requests";
          if (arrayView ? !Array.isArray(value) : Array.isArray(value) || typeof (value as { capturedAt?: unknown }).capturedAt !== "string") throw new Error("Invalid live state");
          delay = 500;
          snapshot(value);
          connection(true);
        }
      } catch {
        socket?.close();
      }
    };
    socket.onclose = () => {
      clearTimeout(watchdog);
      if (stopped) return;
      connection(false);
      retry = setTimeout(start, delay);
      delay = Math.min(delay * 2, 30000);
    };
  };
  start();
  return () => {
    stopped = true;
    clearTimeout(retry);
    clearTimeout(watchdog);
    socket?.close();
  };
}
