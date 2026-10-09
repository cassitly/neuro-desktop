import RuntimePanel from "./RuntimePanel";
import { useCallback, useEffect, useState } from "react";
import GamesPanel from "./GamesPanel";
import PermissionsPage from "./PermissionsPage";
import {
  ApiError,
  adminToken,
  api,
  setAdminToken,
  type CatalogItem,
  type ConfigPayload,
  type InstalledExtension,
  type RelayStatus,
  type StatusPayload,
} from "./api";

type Tab = "extensions" | "games" | "permissions" | "settings";

const INSTALL_MODE_LABELS: Record<string, string> = {
  metadata_only: "Metadata only (records the install)",
  git_clone: "GitHub clone into the plugin directory",
  mcp_endpoint: "MCP endpoint (external server)",
};

export default function App() {
  const [showSplash, setShowSplash] = useState(true);
  const [activeTab, setActiveTab] = useState<Tab>("extensions");
  const [status, setStatus] = useState<StatusPayload | null>(null);
  const [config, setConfig] = useState<ConfigPayload | null>(null);
  const [extensions, setExtensions] = useState<InstalledExtension[]>([]);
  const [catalog, setCatalog] = useState<CatalogItem[]>([]);
  const [installMode, setInstallMode] = useState<string>("");
  const [extensionDir, setExtensionDir] = useState<string>("");
  const [selectedId, setSelectedId] = useState<string>("");
  const [message, setMessage] = useState<{ kind: "ok" | "error"; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [tokenDraft, setTokenDraft] = useState(() => adminToken());

  const load = useCallback(async () => {
    try {
      const [statusPayload, extensionsPayload, configPayload] = await Promise.all([
        api.status(),
        api.extensions(),
        api.config().catch(() => null),
      ]);
      setStatus(statusPayload);
      setExtensions(extensionsPayload.installed);
      setCatalog(extensionsPayload.catalog);
      setInstallMode(extensionsPayload.install_mode);
      setExtensionDir(extensionsPayload.extension_dir);
      setConfig(configPayload);
      setSelectedId((current) => {
        if (current && (extensionsPayload.installed.some((item) => item.id === current) || extensionsPayload.catalog.some((item) => item.id === current))) {
          return current;
        }
        return extensionsPayload.installed[0]?.id ?? extensionsPayload.catalog[0]?.id ?? "";
      });
    } catch (error) {
      setMessage({
        kind: "error",
        text:
          error instanceof ApiError
            ? `${error.message} — the dashboard talks to the Go bridge over HTTP; make sure it is running.`
            : "Could not reach the bridge",
      });
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => setShowSplash(false), 900);
    return () => window.clearTimeout(timer);
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 5000);
    return () => window.clearInterval(timer);
  }, [load]);

  async function extensionAction(id: string, action: "install" | "enable" | "disable" | "uninstall") {
    setBusy(true);
    setMessage(null);
    try {
      const result = await api.extensionAction(id, action);
      setMessage({ kind: "ok", text: result.message || `${action} ok` });
      await load();
    } catch (error) {
      setMessage({
        kind: "error",
        text:
          error instanceof ApiError
            ? `${action} failed: ${error.message}`
            : `${action} failed`,
      });
    } finally {
      setBusy(false);
    }
  }

  const installedById = new Map(extensions.map((item) => [item.id, item]));
  const catalogById = new Map(catalog.map((item) => [item.id, item]));
  const allIds = [
    ...extensions.map((item) => item.id),
    ...catalog.filter((item) => !installedById.has(item.id)).map((item) => item.id),
  ];
  const selected = (
    installedById.get(selectedId) ??
    (catalogById.get(selectedId)
      ? {
          id: selectedId,
          enabled: false,
          name: catalogById.get(selectedId)?.name,
          description: catalogById.get(selectedId)?.description,
          type: catalogById.get(selectedId)?.type,
          repository: catalogById.get(selectedId)?.repository,
          source: "catalog",
        }
      : undefined)
  ) as InstalledExtension | undefined;
  const selectedInstalled = installedById.has(selectedId);

  const core = extensions.filter((item) => item.type === "desktop-integration" || item.id === "neuro-desktop");
  const installedPlugins = extensions.filter((item) => !core.includes(item));

  const relay: RelayStatus | undefined = status?.relay;
  const executorOnline = Boolean(status?.executor.connected);

  if (showSplash) {
    return (
      <div className="splash-screen">
        <div className="splash-overlay" />
        <div className="splash-content">
          <h1>Neuro Desktop</h1>
          <p>Integration Manager</p>
          <span>{status?.version ?? ""}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="manager-root">
      <header className="manager-titlebar">
        <div className="titlebar-left">
          <h1>Neuro Desktop</h1>
          <nav className="titlebar-nav">
            {(["extensions", "games", "permissions", "settings"] as Tab[]).map((tab) => (
              <button
                key={tab}
                className={`nav-tab ${activeTab === tab ? "active" : ""}`}
                onClick={() => setActiveTab(tab)}
              >
                {tab === "settings" ? "Status" : tab[0].toUpperCase() + tab.slice(1)}
              </button>
            ))}
          </nav>
        </div>
        <div className="titlebar-right">
          <span className={`status-indicator ${executorOnline ? "online" : "offline"}`}>●</span>
          <span className="status-text">
            {status
              ? `bridge ${status.version} · executor ${executorOnline ? "online" : "offline"} · ${
                  status.actions.actions_received ?? 0
                } actions`
              : "connecting to the bridge…"}
          </span>
        </div>
      </header>

      {message && (
        <div className={message.kind === "ok" ? "save-notice" : "scope-warning"}>
          {message.kind === "ok" ? "✅" : "⚠️"} {message.text}
        </div>
      )}

      {activeTab === "permissions" && <PermissionsPage />}

      {activeTab === "games" && <GamesPanel executor={status?.executor ?? null} />}

      {activeTab === "extensions" && (
        <>
        <RuntimePanel />
        <main className="manager-main manager-main--extensions">
          <aside className="manager-sidebar">
            <section>
              <h2>Core Services</h2>
              {core.map((item, index) => (
                <button
                  key={item.id}
                  className={`sidebar-item ${selectedId === item.id ? "selected" : ""}`}
                  style={{ animationDelay: `${index * 0.04}s` }}
                  onClick={() => setSelectedId(item.id)}
                >
                  <div>
                    <strong>{item.name || item.id}</strong>
                    <small>{item.enabled ? "Active" : "Installed"}</small>
                  </div>
                </button>
              ))}
            </section>

            <section>
              <h2>Extensions</h2>
              {installedPlugins.map((item, index) => (
                <button
                  key={item.id}
                  className={`sidebar-item ${selectedId === item.id ? "selected" : ""}`}
                  style={{ animationDelay: `${index * 0.05 + 0.12}s` }}
                  onClick={() => setSelectedId(item.id)}
                >
                  <div>
                    <strong>{item.name || item.id}</strong>
                    <small>
                    {item.enabled ? "Enabled" : "Disabled"} · signature {item.signature_state ?? item.trust ?? "unknown"}
                  </small>
                  </div>
                </button>
              ))}
              {catalog
                .filter((item) => !installedById.has(item.id))
                .map((item) => (
                  <button
                    key={item.id}
                    className={`sidebar-item ${selectedId === item.id ? "selected" : ""}`}
                    onClick={() => setSelectedId(item.id)}
                  >
                    <div>
                      <strong>{item.name}</strong>
                      <small>Not installed · signature {item.signature_state ?? "unknown"}</small>
                    </div>
                  </button>
                ))}
              {allIds.length === 0 && (
                <p className="hint">
                  No catalog entries. Check the catalog path in Status.
                </p>
              )}
            </section>
          </aside>

          <section className="manager-panel">
            {selected ? (
              <>
                <div className="panel-header">
                  <div>
                    <h1>{selected.name || selected.id}</h1>
                    <p>{selected.description || "No description in the catalog."}</p>
                  </div>
                  {selected.repository && (
                    <a href={selected.repository} target="_blank" rel="noreferrer">
                      Open Repository ↗
                    </a>
                  )}
                </div>

                <div className="status-row">
                  <span className={`pill ${selectedInstalled ? "ok" : "warn"}`}>
                    {selectedInstalled ? "Installed" : "Not Installed"}
                  </span>
                  <span className={`pill ${selected.enabled ? "ok" : "idle"}`}>
                    {selected.enabled ? "Enabled" : "Disabled"}
                  </span>
                  <span className="pill neutral">{INSTALL_MODE_LABELS[installMode] ?? installMode}</span>
                </div>

                <div className="button-row">
                  <button
                    className="primary"
                    onClick={() => void extensionAction(selected.id, "install")}
                    disabled={busy || selectedInstalled}
                  >
                    Install
                  </button>
                  <button
                    className="secondary"
                    onClick={() =>
                      void extensionAction(selected.id, selected.enabled ? "disable" : "enable")
                    }
                    disabled={busy || !selectedInstalled}
                  >
                    {selected.enabled ? "Disable" : "Enable"}
                  </button>
                  <button
                    className="danger"
                    onClick={() => void extensionAction(selected.id, "uninstall")}
                    disabled={busy || !selectedInstalled}
                  >
                    Uninstall
                  </button>
                </div>

                <div className="meta-grid">
                  <article>
                    <h3>Installed At</h3>
                    <p>{selected.installed_at || "not installed"}</p>
                  </article>
                  <article>
                    <h3>Source</h3>
                    <p>{selected.source || "—"}</p>
                  </article>
                  <article>
                    <h3>Path</h3>
                    <p>{selected.path || extensionDir}</p>
                  </article>
                  <article>
                    <h3>Type</h3>
                    <p>{selected.type || "extension"}</p>
                  </article>
                </div>

                <p className="hint">
                  Extensions are recorded in the bridge's extension state file and listed to Neuro as
                  installed plugins. Installation obeys the install mode above and the filesystem
                  permission scope.
                </p>
              </>
            ) : (
              <div className="panel-header">
                <div>
                  <h1>Nothing selected</h1>
                  <p>Pick an extension on the left, or add one to the catalog.</p>
                </div>
              </div>
            )}
          </section>
        </main>
        </>
      )}

      {activeTab === "settings" && (
        <section className="settings-panel">
          <h1>Bridge Status</h1>
          <div className="settings-grid">
            <article>
              <h3>Runtime</h3>
              <p>Version {status?.version ?? "unknown"}</p>
              <p>Uptime {status ? Math.round(status.uptime / 60) : 0} minutes</p>
              <p>Started {status?.started_at ?? "—"}</p>
            </article>
            <article>
              <h3>Executor</h3>
              <p>{executorOnline ? `Connected (${status?.executor.remote})` : "Not connected"}</p>
              <p>
                {status?.executor.completed_commands ?? 0} commands, {status?.executor.failed_commands ?? 0} failed
              </p>
              {status?.executor.last_error && <p className="denied">{status.executor.last_error}</p>}
            </article>
            <article>
              <h3>Relay / coexistence</h3>
              <p>{relay?.enabled ? `Enabled → ${relay.url}` : "Disabled"}</p>
              <p>
                {relay?.registered ? "Registered" : "Not registered"} · {relay?.peer_count ?? 0} peer(s)
              </p>
              {relay?.peers && Object.keys(relay.peers).length > 0 && (
                <ul className="list-items">
                  {Object.entries(relay.peers).map(([name, kind]) => (
                    <li key={name}>
                      <code>{name}</code>
                      <span>{kind}</span>
                    </li>
                  ))}
                </ul>
              )}
              {relay?.reserved_actions && relay.reserved_actions.length > 0 && (
                <p>Reserved for other integrations: {relay.reserved_actions.join(", ")}</p>
              )}
              {relay?.last_error && <p className="denied">{relay.last_error}</p>}
            </article>
            <article>
              <h3>Paths</h3>
              {config &&
                Object.entries(config.paths).map(([name, path]) => (
                  <p key={name}>
                    <strong>{name}:</strong> {path}
                  </p>
                ))}
            </article>
            <article>
              <h3>Admin token</h3>
              <p>
                {status?.admin.token_required
                  ? "This bridge requires a token for changes."
                  : "No token configured (loopback only)."}
              </p>
              <div className="list-input-row">
                <input
                  type="password"
                  value={tokenDraft}
                  onChange={(event) => setTokenDraft(event.target.value)}
                  placeholder="NEURO_ADMIN_TOKEN"
                />
                <button
                  onClick={() => {
                    setAdminToken(tokenDraft.trim());
                    setMessage({ kind: "ok", text: "Token stored for this browser session." });
                  }}
                >
                  Use
                </button>
              </div>
            </article>
            <article>
              <h3>Features</h3>
              {config &&
                Object.entries(config.features).map(([name, enabled]) => (
                  <p key={name}>
                    <strong>{name}:</strong> {enabled ? "on" : "off"}
                  </p>
                ))}
            </article>
          </div>
        </section>
      )}
    </div>
  );
}
