mod controller;
mod go_manager;
mod ipc_handler;
mod relay_manager;
mod ui_launcher;

use controller::Controller;
use go_manager::GoProcessManager;
use ipc_handler::IPCHandler;
use relay_manager::RelayProcessManager;
use serde::Deserialize;
use std::env;
use std::fs;
use std::path::PathBuf;

#[derive(Debug, Deserialize)]
struct IntegrationConfig {
    connection: ConnectionConfig,
}

#[derive(Debug, Deserialize)]
struct ConnectionConfig {
    #[serde(rename = "neuro-backend")]
    neuro_backend: String,
}

fn load_config() -> Result<IntegrationConfig, Box<dyn std::error::Error>> {
    let exe_dir = env::current_exe()?
        .parent()
        .ok_or("No parent dir")?
        .to_path_buf();

    let config_path = exe_dir.join("config").join("integration-config.yml");

    if !config_path.exists() {
        let dev_config = PathBuf::from("config/integration-config.yml");
        if dev_config.exists() {
            let content = fs::read_to_string(dev_config)?;
            return Ok(serde_yaml::from_str(&content)?);
        }
    }

    let content = fs::read_to_string(config_path)?;
    Ok(serde_yaml::from_str(&content)?)
}

fn default_ipc_path() -> String {
    env::current_exe()
        .ok()
        .and_then(|p| {
            p.parent()
                .map(|p| p.join("neuro-integration-code-ipc.json"))
        })
        .unwrap_or_else(|| PathBuf::from("./neuro-integration-code-ipc.json"))
        .to_string_lossy()
        .to_string()
}

fn default_permissions_path() -> String {
    env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|p| p.join("permissions.json")))
        .unwrap_or_else(|| PathBuf::from("./permissions.json"))
        .to_string_lossy()
        .to_string()
}

fn env_bool(name: &str, default: bool) -> bool {
    env::var(name)
        .map(|v| matches!(v.to_lowercase().as_str(), "1" | "true" | "yes" | "on"))
        .unwrap_or(default)
}

fn arg_present(target: &str) -> bool {
    env::args().any(|arg| arg == target)
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    println!("=======================================================");
    println!("           Neuro Desktop Control System");
    println!("=======================================================");
    println!();

    let config = load_config().unwrap_or_else(|e| {
        eprintln!("Warning: Could not load config file: {}", e);
        eprintln!("Using default values...");
        IntegrationConfig {
            connection: ConnectionConfig {
                neuro_backend: "ws://localhost:8000".to_string(),
            },
        }
    });

    let backend_ws_url =
        env::var("NEURO_SDK_WS_URL").unwrap_or_else(|_| config.connection.neuro_backend.clone());
    let ipc_path = env::var("NEURO_IPC_FILE").unwrap_or_else(|_| default_ipc_path());
    let permissions_path =
        env::var("NEURO_PERMISSIONS_FILE").unwrap_or_else(|_| default_permissions_path());

    let relay_enabled = env_bool("NEURO_RELAY_ENABLED", false);
    let supervised_mode = env_bool("NEURO_SUPERVISED", false) || arg_present("--supervised");
    let relay_name =
        env::var("NEURO_RELAY_NAME").unwrap_or_else(|_| "Neuro Desktop Hub".to_string());
    let relay_emulated_addr =
        env::var("NEURO_RELAY_EMULATED_ADDR").unwrap_or_else(|_| "127.0.0.1:8001".to_string());

    let integration_ws_url = if relay_enabled {
        format!("ws://{}", relay_emulated_addr)
    } else {
        backend_ws_url.clone()
    };

    println!("Configuration:");
    println!("  - Neuro Backend WS: {}", backend_ws_url);
    println!("  - Integration WS:   {}", integration_ws_url);
    println!("  - IPC File:         {}", ipc_path);
    println!("  - Permissions File: {}", permissions_path);
    println!("  - Relay Enabled:    {}", relay_enabled);
    println!("  - Supervised Mode:  {}", supervised_mode);
    if relay_enabled {
        println!("  - Relay Name:       {}", relay_name);
        println!("  - Relay Addr:       {}", relay_emulated_addr);
    }
    println!();

    println!("[1/5] Initializing Python controller drivers...");
    let controller =
        Controller::initialize_drivers().expect("Failed to initialize controller drivers");
    println!("      [ok] Python drivers loaded");
    println!();

    println!("[2/5] Initializing optional relay process...");
    let mut relay_manager = if !supervised_mode && relay_enabled {
        Some(RelayProcessManager::new().expect("Failed to create relay manager"))
    } else {
        None
    };
    println!("      [ok] Relay initialization complete");
    println!();

    println!("[3/5] Initializing Neuro integration process...");
    let mut go_manager = if !supervised_mode {
        Some(GoProcessManager::new().expect("Failed to create Go manager"))
    } else {
        None
    };
    println!("      [ok] Integration process manager ready");
    println!();

    println!("[4/5] Starting IPC handler...");
    let ipc = IPCHandler::new(&ipc_path);
    let ipc_handler = ipc.start(controller);
    println!("      [ok] IPC handler running on: {}", ipc_path);
    println!();

    if let Some(relay) = relay_manager.as_mut() {
        println!("[5/5] Starting Neuro relay...");
        relay
            .start(&relay_name, &backend_ws_url, &relay_emulated_addr)
            .expect("Failed to start Neuro relay");
        println!("      [ok] Relay process started");
        println!();
    }

    if let Some(manager) = go_manager.as_mut() {
        println!("[5/5] Starting Neuro integration...");
        manager
            .start(&integration_ws_url, &ipc_path, &permissions_path)
            .expect("Failed to start Go integration");
        println!("      [ok] Integration process started");
    } else {
        println!("[5/5] Integration launch delegated to process-handler (--supervised)");
    }
    println!();

    ui_launcher::launch_ui_if_enabled();

    println!("=======================================================");
    println!("Neuro Desktop is ready.");
    println!("Press Ctrl+C to stop.");
    println!("=======================================================");
    println!();

    let mut check_interval = tokio::time::interval(tokio::time::Duration::from_secs(5));

    loop {
        tokio::select! {
            _ = check_interval.tick() => {
                if !ipc_handler.load(std::sync::atomic::Ordering::SeqCst) {
                    println!();
                    println!("Shutdown signal received from IPC handler.");
                    if let Some(manager) = go_manager.as_mut() {
                        manager.stop();
                    }
                    if let Some(relay) = relay_manager.as_mut() {
                        relay.stop();
                    }
                    break;
                }

                if let Some(relay) = relay_manager.as_mut() {
                    if !relay.is_running() {
                        eprintln!("[warn] Neuro relay crashed, restarting...");
                        if let Err(e) = relay.restart(&relay_name, &backend_ws_url, &relay_emulated_addr) {
                            eprintln!("[err] Failed to restart Neuro relay: {}", e);
                            break;
                        }
                        println!("[ok] Neuro relay restarted");
                    }
                }

                if let Some(manager) = go_manager.as_mut() {
                    if !manager.is_running() {
                        eprintln!("[warn] Neuro integration crashed, restarting...");
                        if let Err(e) = manager.restart(&integration_ws_url, &ipc_path, &permissions_path) {
                            eprintln!("[err] Failed to restart Neuro integration: {}", e);
                            break;
                        }
                        println!("[ok] Neuro integration restarted");
                    }
                }
            }

            _ = tokio::signal::ctrl_c() => {
                println!();
                println!("Shutting down...");
                if let Some(manager) = go_manager.as_mut() {
                    manager.stop();
                }
                if let Some(relay) = relay_manager.as_mut() {
                    relay.stop();
                }
                break;
            }
        }
    }

    println!("Neuro Desktop stopped.");
    Ok(())
}
