export type Host = {
  id: string;
  name: string;
  address: string;
  port: number;
  map: string;
  desiredState: "running" | "stopped";
  observedState: string;
  activeMap: string;
  status: string;
  revision: number;
  actions: { start: boolean; stop: boolean; changeMap: boolean };
  driver: string;
  error: string;
  lastSeen: string | null;
  online: boolean;
};

export type Workspace = { capturedAt: string; hosts: Host[]; maps: string[] };

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function request<T>(
  token: string,
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`/api${path}`, {
    signal: AbortSignal.timeout(10000),
    ...options,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...options.headers,
    },
  });
  const data = await response
    .json()
    .catch(() => ({ error: "The API returned an invalid response" }));
  if (!response.ok)
    throw new APIError(response.status, data.error || "Request failed");
  return data as T;
}

export function connection(host: Host): string {
  return `${host.address.includes(":") ? `[${host.address}]` : host.address}:${host.port}`;
}
