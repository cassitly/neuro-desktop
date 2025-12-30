// ============================================================
// desktop/apps/neuro-desktop/src/main.rs
// ============================================================

mod controller;
mod ipc_handler;
mod go_manager;

use controller::Controller;
use ipc_handler::IPCHandler;
use go_manager::GoProcessManager;
use std::env;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    println!("=======================================================");
    println!("           Neuro Desktop Control System");
    println!("=======================================================");
    println!();

    // Configuration
    let ws_url = env::var("NEURO_SDK_WS_URL")
        .unwrap_or_else(|_| "ws://localhost:8000".to_string());
    
    let ipc_path = env::var("NEURO_IPC_FILE")
        .unwrap_or_else(|_| {
            env::current_exe()
                .ok()
                .and_then(|p| p.parent().map(|p| p.join("neuro_ipc.json")))
                .unwrap_or_else(|| std::path::PathBuf::from("./neuro_ipc.json"))
                .to_string_lossy()
                .to_string()
        });

    println!("Configuration:");
    println!("  - Neuro WebSocket: {}", ws_url);
    println!("  - IPC File:        {}", ipc_path);
    println!();

    // Initialize Python controller drivers
    println!("[1/3] Initializing Python controller drivers...");
    let controller = Controller::initialize_drivers()
        .expect("Failed to initialize controller drivers");
    println!("      ✓ Python drivers loaded");
    println!();

    // Start IPC handler
    println!("[2/3] Starting IPC handler...");
    let ipc = IPCHandler::new(&ipc_path);
    ipc.start(controller);
    println!("      ✓ IPC handler running on: {}", ipc_path);
    println!();

    // Start Go integration
    println!("[3/3] Starting Go WebSocket integration...");
    let mut go_manager = GoProcessManager::new()
        .expect("Failed to create Go manager");
    
    go_manager.start(&ws_url, &ipc_path)
        .expect("Failed to start Go integration");
    println!("      ✓ Go integration connected to Neuro");
    println!();

    println!("=======================================================");
    println!("  Neuro Desktop is ready!");
    println!("  Neuro can now control your computer.");
    println!("=======================================================");
    println!();
    println!("Press Ctrl+C to stop");
    println!();

    // Monitor Go process and restart if needed
    let mut check_interval = tokio::time::interval(tokio::time::Duration::from_secs(5));
    
    loop {
        tokio::select! {
            _ = check_interval.tick() => {
                if !go_manager.is_running() {
                    eprintln!("⚠ Go integration crashed! Attempting restart...");
                    if let Err(e) = go_manager.restart(&ws_url, &ipc_path) {
                        eprintln!("✗ Failed to restart Go integration: {}", e);
                        break;
                    }
                    println!("✓ Go integration restarted");
                }
            }

            _ = tokio::signal::ctrl_c() => {
                println!();
                println!("Shutting down...");
                go_manager.stop();
                break;
            }
        }
    }

    println!("Neuro Desktop stopped");
    Ok(())
}