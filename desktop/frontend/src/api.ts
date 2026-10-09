/**
 * Neuro Desktop dashboard API client.
 *
 * The dashboard is served by the Go bridge itself (http://127.0.0.1:8300/ui/),
 * so every call is same-origin and relative. When the UI is served from the Vite
 * dev server, `vite.config.ts` proxies /api and /health to the bridge.
 *
 * Every API call needs the dashboard token, including reads and including calls
 * from this machine. The operator gets the token from `neuro-integration setup`
 * and types it into the sign-in page. It is kept in sessionStorage for this
 * browser session only, and it is sent in the X-ND-Token header. The bridge never
 * puts it in a page, and it is not read from the URL.
 */

export type ScopeName =
  | "input"
  | "game"
  | "filesystem"
  | "process"
  | "network"
  | "system"
  | "vision"
  | "shell"
  | "extensions";

export type ScopeConfig = {
  allowed: boolean;
  /** Neuro may ask the operator to switch this scope on (request_permission). */
  requestable?: boolean;
  limits?: {
    /** 0 or missing means unlimited. Enforced by the bridge. */
    max_actions_per_minute?: number;
  };
};

export type PermissionPolicyFile = {
  version?: string | number;
  default_allow: boolean;
  allowed_actions: string[];
  denied_actions: string[];
  scopes: Partial<Record<ScopeName, ScopeConfig>>;
  created_at?: string;
  metadata?: Record<string, string>;
};

export type BootstrapInfo = {
  version?: string;
  api_base?: string;
  nativeHost?: boolean;
  platform?: string;
};

export type ExecutorInfo = {
  connected: boolean;
  remote?: string;
  version?: string;
  completed_commands: number;
  failed_commands: number;
  last_error?: string;
  last_seen?: string;
  total_connections: number;
};

export type BridgeStats = {
  actions_received?: number;
  actions_completed?: number;
  actions_failed?: number;
  actions_denied?: number;
  last_action?: string;
  last_error?: string;
};

export type RelayStatus = {
  enabled: boolean;
  url?: string;
  connected: boolean;
  registered: boolean;
  peer_count: number;
  peers?: Record<string, string>;
  last_error?: string;
  last_event?: string;
  last_seen?: string;
  reserved_actions?: string[];
};

export type GameProfileSummary = {
  id: string;
  name: string;
  description?: string;
  mode: string;
  external?: string;
  keys?: Record<string, string>;
  mouse_look: boolean;
  vision: boolean;
  launchable: boolean;
  tags?: string[];
};

export type GameSession = {
  profile_id: string;
  profile_name: string;
  control_mode: string;
  external_integration?: string;
  started_at: string;
  actions_issued: number;
  last_action?: string;
  last_action_at?: string;
};

export type GameDetected = {
  profile?: GameProfileSummary;
  matched_on?: string;
  active_window?: string;
  process_candidates?: string[];
};

export type StatusPayload = {
  ok: boolean;
  version: string;
  integration: string;
  uptime: number;
  started_at: string;
  executor: ExecutorInfo;
  negotiation: {
    executor_connected: boolean;
    file_ipc_path: string;
    protocol_version: string;
  };
  permissions_path: string;
  game: {
    registry_source: string;
    profiles: number;
    session: GameSession | null;
  };
  relay: RelayStatus;
  actions: BridgeStats;
  admin: {
    listen: string;
    token_required: boolean;
    token_configured?: boolean;
    token_source?: string;
    requests: number;
    last_error?: string;
  };
};

export type CatalogItem = {
  id: string;
  name: string;
  description?: string;
  type?: string;
  repository?: string;
  homepage?: string;
  tags?: string[];
  launch_hints?: string[];
  publisher?: string;
  commit?: string;
  /** verified | unsigned | untrusted | invalid (see catalog_trust.go). */
  signature_state?: string;
  signature_detail?: string;
  mcp?: { command: string; args?: string[] };
};

export type InstalledExtension = {
  id: string;
  enabled: boolean;
  installed_at?: string;
  source?: string;
  path?: string;
  name?: string;
  description?: string;
  type?: string;
  repository?: string;
  trust?: string;
  signature_state?: string;
};

export type PermissionRequest = {
  id: string;
  scope: ScopeName;
  reason: string;
  minutes: number;
  status: "pending" | "approved" | "denied";
  granted_minutes?: number;
  requested_at: string;
  decided_at?: string;
  granted_until?: string;
  note?: string;
};

export type PermissionGrant = {
  scope: ScopeName;
  granted_at: string;
  /** Empty or missing means the grant lasts until it is revoked. */
  expires_at?: string;
  reason?: string;
  request_id?: string;
};

export type PermissionRequestsPayload = {
  pending: PermissionRequest[];
  recent: PermissionRequest[];
  grants: PermissionGrant[];
  requestable: ScopeName[];
};

export type RuntimeState = {
  state: string;
  [key: string]: unknown;
};

export type RuntimePayload = {
  ok: boolean;
  checked_at: string;
  bridge: RuntimeState & { uptime_seconds: number; admin_listen?: string; paused?: boolean };
  executor: RuntimeState & { connected: boolean };
  relay: RuntimeState & { enabled: boolean; url?: string; peer_count: number; last_error?: string };
  vision: RuntimeState & { configured: boolean; url?: string; backend?: string; latency_ms?: number; error?: string };
  mcp: RuntimeState & {
    running: number;
    servers: { id: string; state: string; tools?: string[]; last_error?: string; trust?: string }[];
  };
};

export type ExtensionsPayload = {
  ok: boolean;
  installed: InstalledExtension[];
  catalog: CatalogItem[];
  install_mode: string;
  extension_dir: string;
  state_file: string;
};

export type ActionInfo = {
  name: string;
  scope: string;
  description?: string;
  allowed: boolean;
  registered: boolean;
  reserved: boolean;
  kind?: string;
};

export type ActionsPayload = {
  actions: ActionInfo[];
  registered: string[];
  reserved: string[];
  registered_count: number;
  reserved_count: number;
};

export type PermissionSchema = {
  scopes: (ScopeName | { name: ScopeName; title?: string; description?: string })[];
  actions: { name: string; scope: string; kind?: string }[];
  defaults: { default_allow: boolean };
};

export type ConfigPayload = {
  ok: boolean;
  version: string;
  paths: Record<string, string>;
  features: Record<string, boolean>;
};

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

const TOKEN_KEY = "nd_admin_token";

function bootstrap(): BootstrapInfo | undefined {
  return window.__ND_BOOTSTRAP;
}

/** The token the operator signed in with, for this browser session only. */
export function adminToken(): string {
  try {
    return window.sessionStorage.getItem(TOKEN_KEY) ?? "";
  } catch {
    return "";
  }
}

export function setAdminToken(token: string) {
  try {
    if (token) {
      window.sessionStorage.setItem(TOKEN_KEY, token);
    } else {
      window.sessionStorage.removeItem(TOKEN_KEY);
    }
  } catch {
    // Storage can be unavailable (private mode); the token just won't persist.
  }
}

/** Same-origin by default; NEURO_UI_API / __ND_BOOTSTRAP.api_base override it. */
export const API_BASE = (bootstrap()?.api_base ?? "").replace(/\/$/, "");

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const token = adminToken();
  if (token) {
    headers.set("X-ND-Token", token);
  }

  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, { ...init, headers });
  } catch (error) {
    throw new ApiError(0, `Cannot reach the Neuro Desktop bridge (${path}). Is it running?`);
  }

  const text = await response.text();
  let payload: any = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = { error: text };
    }
  }

  if (!response.ok) {
    const message =
      (payload && (payload.error || payload.message)) || `Request failed (HTTP ${response.status})`;
    throw new ApiError(response.status, message);
  }

  return payload as T;
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: body === undefined ? "" : JSON.stringify(body),
  });
}

export const api = {
  /** Checks the token without reading anything else. 401 means it is wrong; 503 means setup has not run. */
  session: () => request<{ ok: boolean; version: string; token_source: string }>("/api/session"),
  status: () => request<StatusPayload>("/api/status"),
  config: () => request<ConfigPayload>("/api/config"),

  permissions: () => request<PermissionPolicyFile>("/api/permissions"),
  permissionSchema: () => request<PermissionSchema>("/api/permissions/schema"),
  savePermissions: (policy: PermissionPolicyFile) =>
    request<{ ok: boolean; saved: string; applied_live: boolean }>("/api/permissions", {
      method: "PUT",
      body: JSON.stringify(policy),
    }),

  actions: () => request<ActionsPayload>("/api/actions"),

  extensions: () => request<ExtensionsPayload>("/api/extensions"),
  extensionAction: (id: string, action: "install" | "enable" | "disable" | "uninstall") =>
    post<{ ok: boolean; message?: string }>(
      `/api/extensions/${encodeURIComponent(id)}/${action}`,
    ),

  games: () =>
    request<{
      ok: boolean;
      profiles: GameProfileSummary[];
      registry_source: string;
      detected: GameDetected | null;
      session: GameSession | null;
    }>("/api/games"),

  startSession: (profileId: string, launch: boolean, mode: string) =>
    post<{ ok: boolean; message: string; session: GameSession | null }>("/api/games/session", {
      profile_id: profileId,
      launch,
      mode,
    }),

  endSession: () =>
    request<{ ok: boolean; message: string }>("/api/games/session", { method: "DELETE" }),

  releaseInput: () => post<{ ok: boolean; message: string }>("/api/games/release"),

  observe: (vision = true) =>
    post<{ ok: boolean; observation: string }>("/api/games/observe", { vision }),

  relay: () => request<{ ok: boolean; relay: RelayStatus }>("/api/relay"),

  runtime: () => request<RuntimePayload>("/api/runtime"),

  permissionRequests: () => request<PermissionRequestsPayload>("/api/permission-requests"),
  decidePermissionRequest: (id: string, approve: boolean, minutes?: number) =>
    post<{ ok: boolean; message?: string }>(
      `/api/permission-requests/${encodeURIComponent(id)}/${approve ? "approve" : "deny"}`,
      approve && minutes !== undefined ? { minutes } : {},
    ),
  revokePermissionGrant: (scope: ScopeName) =>
    post<{ ok: boolean; message?: string }>(
      `/api/permissions/grants/${encodeURIComponent(scope)}/revoke`,
    ),
};
