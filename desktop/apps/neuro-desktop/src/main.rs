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
    println!("[1/4] Initializing Python controller drivers...");
    let controller = Controller::initialize_drivers()
        .expect("Failed to initialize controller drivers");
    println!("      ✓ Python drivers loaded");
    println!();

    // Initialize Go process manager
    println!("[2/4] Initializing Neuro integration Integration...");
    let mut go_manager = GoProcessManager::new()
        .expect("Failed to create Go manager");
    println!("      ✓ Go integration ready");

    // Start IPC handler
    println!("[3/4] Starting IPC handler...");
    let ipc = IPCHandler::new(&ipc_path);
    ipc.start(controller);
    println!("      ✓ IPC handler running on: {}", ipc_path);
    println!();

    // Start Go integration
    println!("[4/4] Starting Neuro Integration Code...");
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
                if !ipc.is_running() { // check for this first, to hopefully avoid a race condition
                    // where go_manager might try to restart, using the code below this if statement.
                    println!(); // Print out a space, so that this message can be seen more clearly
                    println!("Shutdown signal received, Neuro Desktop is stopping fully...");
                    go_manager.stop();
                    break;
                }

                if !go_manager.is_running() {
                    eprintln!("⚠ Neuro integration crashed! Attempting restart...");
                    if let Err(e) = go_manager.restart(&ws_url, &ipc_path) {
                        eprintln!("✗ Failed to restart Neuro integration: {}", e);
                        break;
                    }
                    println!("✓ Neuro integration restarted");
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