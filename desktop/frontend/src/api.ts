/**
 * Neuro Desktop dashboard API client.
 *
 * The dashboard is served by the Go bridge itself (http://127.0.0.1:8300/ui/),
 * so every call is same-origin and relative. When the UI is served from the Vite
 * dev server, `vite.config.ts` proxies /api and /health to the bridge.
 *
 * Writes are guarded by the admin token (`NEURO_ADMIN_TOKEN`). The shell pages
 * served by the bridge inject it as `window.__ND_BOOTSTRAP.token` for local
 * browsers; otherwise the operator pastes it into the Settings tab and it is
 * kept in sessionStorage only.
 */

export type ScopeName =
  | "input"
  | "game"
  | "filesystem"
  | "process"
  | "network"
  | "system"
  | "vision";

export type ScopeConfig = {
  allowed: boolean;
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
  token?: string;
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
  process_restarts: number;
  last_error?: string;
  last_event?: string;
  last_seen?: string;
  process_managed_here: boolean;
  reserved_actions?: string[];
  supervised_by?: string;
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

/**
 * The token from the injected bootstrap, then a `?token=` in the URL (which is
 * stored and stripped so a reload keeps working), then whatever the operator
 * typed into the Settings tab.
 */
export function adminToken(): string {
  const injected = bootstrap()?.token;
  if (injected) {
    return injected;
  }

  try {
    const fromUrl = new URLSearchParams(window.location.search).get("token");
    if (fromUrl) {
      window.sessionStorage.setItem(TOKEN_KEY, fromUrl);
      const url = new URL(window.location.href);
      url.searchParams.delete("token");
      window.history.replaceState({}, "", url.toString());
      return fromUrl;
    }
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
};
