//! Opens Vedal's dashboard in the default browser.
//!
//! The old implementation opened `frontend/index.html` directly, i.e. a
//! `file://` page. A file:// page has a null origin, so it cannot call the
//! bridge API on `http://127.0.0.1:8300`, which is why the shipped UI was
//! reduced to localStorage and had no live permissions or plugins. The bridge
//! now serves the compiled UI itself, so the launcher opens that URL.

use std::env;
use std::process::Command;

fn env_bool(name: &str, default: bool) -> bool {
    env::var(name)
        .map(|value| matches!(value.to_lowercase().as_str(), "1" | "true" | "yes" | "on"))
        .unwrap_or(default)
}

/// Resolves the dashboard URL the launcher should open, e.g.
/// `http://127.0.0.1:8300/ui/` (or `https://…` when the admin API is behind TLS).
pub fn dashboard_url(admin_listen: &str) -> String {
    if let Ok(explicit) = env::var("NEURO_UI_URL") {
        let trimmed = explicit.trim().to_string();
        if !trimmed.is_empty() {
            return trimmed;
        }
    }

    let listen = admin_listen.trim();
    if listen.is_empty() {
        return "http://127.0.0.1:8300/ui/".to_string();
    }

    // "0.0.0.0:8300" is a bind address, not a reachable host.
    let host_port = if let Some(rest) = listen.strip_prefix("0.0.0.0:") {
        format!("127.0.0.1:{rest}")
    } else if let Some(rest) = listen.strip_prefix("[::]:") {
        format!("127.0.0.1:{rest}")
    } else {
        listen.to_string()
    };

    format!("http://{host_port}/ui/")
}

pub fn launch_ui_if_enabled(admin_listen: &str) {
    if !env_bool("NEURO_UI_LAUNCH", true) {
        return;
    }

    let url = dashboard_url(admin_listen);
    println!("[ui] Dashboard: {url}");

    if env_bool("NEURO_UI_OPEN_BROWSER", true) {
        open_in_browser(&url);
    }
}

fn open_in_browser(url: &str) {
    #[cfg(target_os = "windows")]
    {
        // `start` is a cmd builtin; the empty argument is the window title.
        let _ = Command::new("cmd").args(["/C", "start", "", url]).spawn();
    }

    #[cfg(target_os = "macos")]
    {
        let _ = Command::new("open").arg(url).spawn();
    }

    #[cfg(all(unix, not(target_os = "macos")))]
    {
        match Command::new("xdg-open").arg(url).spawn() {
            Ok(_) => {}
            Err(err) => {
                eprintln!("[ui] Could not open a browser ({err}); visit {url} manually");
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::dashboard_url;

    #[test]
    fn binds_to_loopback_for_wildcard_addresses() {
        assert_eq!(dashboard_url("0.0.0.0:8300"), "http://127.0.0.1:8300/ui/");
        assert_eq!(dashboard_url("127.0.0.1:9000"), "http://127.0.0.1:9000/ui/");
    }

    #[test]
    fn empty_address_falls_back_to_the_default_dashboard() {
        assert_eq!(dashboard_url(""), "http://127.0.0.1:8300/ui/");
    }
}
