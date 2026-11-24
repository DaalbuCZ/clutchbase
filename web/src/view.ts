import { useEffect, useState } from "react";

export function viewHash(view: string, entityId = ""): string {
  const key = view === "matches" ? "room" : view === "tournaments" ? "tournament" : view === "servers" || view === "access" ? "server" : "";
  return key && entityId ? `#${view}?${key}=${encodeURIComponent(entityId)}` : `#${view}`;
}

// Routes select a view or entity; authoritative application state remains in snapshots.
export function useView<T extends string>(views: readonly T[], fallback: T): [T, (view: T, entityId?: string) => void, string] {
  const read = () => {
    const [name, query = ""] = window.location.hash.slice(1).split("?", 2);
    const view = views.find((candidate) => candidate === name) ?? fallback;
    const key = view === "matches" ? "room" : view === "tournaments" ? "tournament" : view === "servers" || view === "access" ? "server" : "";
    return { view, entityId: key ? new URLSearchParams(query).get(key) || "" : "" };
  };
  const [route, setRoute] = useState(read);
  useEffect(() => {
    const onChange = () => setRoute(read());
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, [views, fallback]);
  return [route.view, (next, entityId = "") => {
    const key = next === "matches" ? "room" : next === "tournaments" ? "tournament" : next === "servers" || next === "access" ? "server" : "";
    const hash = viewHash(next, entityId);
    if (window.location.hash !== hash) window.location.hash = hash;
    setRoute({ view: next, entityId: key ? entityId : "" });
  }, route.entityId];
}
