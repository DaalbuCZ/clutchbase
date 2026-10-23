# Clutchbase

Read `docs/PROJECT.md` before planning or changing the product. It records the
user's decisions, roadmap, licensing constraints, and current implementation.
Read `README.md` for development commands and operational limitations.

- Stack: React + TypeScript, Go, PostgreSQL. Keep one repository.
- Build independent application and host-agent source. Do not copy or depend on
  Auto-Tournament, its cs2-server-manager, Ready Up, or their restricted forks.
- Prioritize playing with friends and server management. Tournaments are optional later.
- Prefer small complete vertical slices and standard-library Go HTTP handlers.
- Do not add pointless subheadings, eyebrow labels, marketing taglines, or
  repeated headings. Keep UI and documentation copy direct; use headings only
  when they meaningfully organize content. Keep useful field and status labels.
- PostgreSQL-backed server state is authoritative. The UI renders canonical
  snapshots and server-supplied actions; it must not keep a second lifecycle
  model or optimistically mark a server running/stopped.
- Never send arbitrary shell commands or host paths from the browser to the agent.
- Keep agent and administrator credentials separate. Never log them.
- Update `docs/PROJECT.md` when decisions, implemented scope, or limitations change.
- Run Go tests, frontend type checking/build, and database integration tests for
  changes to the control loop. Simulation does not prove real CS2 compatibility.
