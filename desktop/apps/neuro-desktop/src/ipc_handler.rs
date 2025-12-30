// desktop/apps/neuro-desktop/src/ipc_handler.rs
use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;
use std::thread;
use std::time::Duration;

use crate::controller::Controller;

#[derive(Debug, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum IPCCommand {
    MouseMove { params: MouseMoveParams },
    MouseClick { params: MouseClickParams },
    KeyPress { params: KeyPressParams },
    KeyType { params: KeyTypeParams },
    RunScript { params: RunScriptParams },
}

#[derive(Debug, Deserialize)]
pub struct MouseMoveParams {
    pub x: i32,
    pub y: i32,
}

#[derive(Debug, Deserialize)]
pub struct MouseClickParams {
    pub x: Option<i32>,
    pub y: Option<i32>,
    pub button: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct KeyPressParams {
    pub key: String,
    pub modifiers: Option<Vec<String>>,
}

#[derive(Debug, Deserialize)]
pub struct KeyTypeParams {
    pub text: String,
}

#[derive(Debug, Deserialize)]
pub struct RunScriptParams {
    pub script: String,
}

#[derive(Debug, Serialize)]
pub struct IPCResponse {
    pub success: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

impl IPCResponse {
    pub fn success() -> Self {
        Self {
            success: true,
            data: None,
            error: None,
        }
    }

    pub fn failure(error: String) -> Self {
        Self {
            success: false,
            data: None,
            error: Some(error),
        }
    }
}

pub struct IPCHandler {
    ipc_file: PathBuf,
    response_file: PathBuf,
}

impl IPCHandler {
    pub fn new(ipc_path: &str) -> Self {
        let ipc_file = PathBuf::from(ipc_path);
        let response_file = PathBuf::from(format!("{}.response", ipc_path));

        Self {
            ipc_file,
            response_file,
        }
    }

    pub fn start(self, controller: Controller) {
        thread::spawn(move || {
            loop {
                if let Err(e) = self.process_once(&controller) {
                    eprintln!("IPC processing error: {}", e);
                }
                thread::sleep(Duration::from_millis(50));
            }
        });
    }

    fn process_once(&self, controller: &Controller) -> Result<()> {
        // Check if command file exists
        if !self.ipc_file.exists() {
            return Ok(());
        }

        // Read command
        let data = fs::read_to_string(&self.ipc_file)?;
        let command: IPCCommand = serde_json::from_str(&data)?;

        // Delete command file immediately
        fs::remove_file(&self.ipc_file)?;

        // Execute command
        let response = self.execute_command(controller, command);

        // Write response
        let response_json = serde_json::to_string(&response)?;
        fs::write(&self.response_file, response_json)?;

        Ok(())
    }

    fn execute_command(&self, controller: &Controller, command: IPCCommand) -> IPCResponse {
        match command {
            IPCCommand::MouseMove { params } => {
                match controller.mouse_move(params.x, params.y) {
                    Ok(_) => {
                        controller.execute_instructions().ok();
                        IPCResponse::success()
                    }
                    Err(e) => IPCResponse::failure(format!("Mouse move failed: {}", e)),
                }
            }

            IPCCommand::MouseClick { params } => {
                // If coordinates provided, move first
                if let (Some(x), Some(y)) = (params.x, params.y) {
                    if let Err(e) = controller.mouse_move(x, y) {
                        return IPCResponse::failure(format!("Mouse move failed: {}", e));
                    }
                }

                // Then click
                match controller.mouse_click(
                    params.x.unwrap_or(0),
                    params.y.unwrap_or(0),
                ) {
                    Ok(_) => {
                        controller.execute_instructions().ok();
                        IPCResponse::success()
                    }
                    Err(e) => IPCResponse::failure(format!("Mouse click failed: {}", e)),
                }
            }

            IPCCommand::KeyPress { params } => {
                // Build script for key press with modifiers
                let script = if let Some(modifiers) = params.modifiers {
                    let mods = modifiers.join(" ");
                    format!("SHORTCUT {} {}", mods, params.key)
                } else {
                    format!("PRESS {}", params.key)
                };

                match controller.run_script(&script) {
                    Ok(_) => IPCResponse::success(),
                    Err(e) => IPCResponse::failure(format!("Key press failed: {}", e)),
                }
            }

            IPCCommand::KeyType { params } => {
                match controller.type_text(&params.text) {
                    Ok(_) => {
                        controller.execute_instructions().ok();
                        IPCResponse::success()
                    }
                    Err(e) => IPCResponse::failure(format!("Type text failed: {}", e)),
                }
            }

            IPCCommand::RunScript { params } => {
                match controller.run_script(&params.script) {
                    Ok(_) => IPCResponse::success(),
                    Err(e) => IPCResponse::failure(format!("Script execution failed: {}", e)),
                }
            }
        }
    }
}