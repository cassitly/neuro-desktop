//! TCP executor client — connects to the Go bridge hub and runs commands locally.
//!
//! Protocol (JSON lines):
//!   client → {"type":"hello","role":"executor","version":"1","token":"..."}
//!   server → {"type":"hello_ack",...} | {"type":"hello_nack","error":"..."}
//!   server → {"type":"command","id":"...","command":{...}}
//!   client → {"type":"result","id":"...","success":true,"data":{...}}
//!
//! The client reconnects on its own: the bridge restarts whenever Vedal edits
//! the permissions file or updates a plugin, and a dropped executor used to stay
//! dropped until someone restarted the app by hand.

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::env;
use std::io::{BufRead, BufReader, Write};
use std::net::TcpStream;
use std::time::Duration;

use crate::controller::Controller;
use crate::ipc_handler::{IPCCommand, IPCHandler};

const PROTOCOL_VERSION: &str = "2";

#[derive(Debug, Serialize, Deserialize)]
struct Envelope {
    #[serde(rename = "type")]
    kind: String,
    #[serde(default)]
    id: Option<String>,
    #[serde(default)]
    command: Option<Value>,
    #[serde(default)]
    success: Option<bool>,
    #[serde(default)]
    data: Option<Value>,
    #[serde(default)]
    error: Option<String>,
    #[serde(default)]
    role: Option<String>,
    #[serde(default)]
    version: Option<String>,
    #[serde(default)]
    token: Option<String>,
}

impl Envelope {
    fn new(kind: &str) -> Self {
        Self {
            kind: kind.to_string(),
            id: None,
            command: None,
            success: None,
            data: None,
            error: None,
            role: None,
            version: None,
            token: None,
        }
    }
}

/// Why a session ended, which decides whether the client retries.
enum SessionEnd {
    /// The bridge asked us to stop (shutdown command) — do not reconnect.
    Requested,
    /// The socket dropped or the bridge closed it — reconnect.
    Disconnected,
}

pub fn run_executor_client(server_addr: &str, controller: Controller) -> Result<()> {
    let token = env::var("NEURO_EXECUTOR_TOKEN")
        .ok()
        .filter(|value| !value.trim().is_empty());

    let reconnect_enabled = env_bool("NEURO_EXECUTOR_RECONNECT", true);
    let max_reconnects = env::var("NEURO_EXECUTOR_MAX_RECONNECTS")
        .ok()
        .and_then(|value| value.trim().parse::<u32>().ok())
        .unwrap_or(0); // 0 = keep trying

    println!("[executor] Mode: EXECUTOR CLIENT — bridge {server_addr}");
    if token.is_some() {
        println!("[executor] Using the shared executor token from NEURO_EXECUTOR_TOKEN");
    }

    let mut attempt: u32 = 0;

    loop {
        match run_session(server_addr, &controller, token.as_deref()) {
            Ok(SessionEnd::Requested) => {
                println!("[executor] Shutdown requested by the bridge — stopping");
                return Ok(());
            }
            Ok(SessionEnd::Disconnected) => {
                println!("[executor] Bridge closed the connection");
            }
            Err(err) => {
                eprintln!("[executor] Session failed: {err:#}");
                if !reconnect_enabled {
                    return Err(err);
                }
                if is_fatal(&err) {
                    eprintln!("[executor] Not retrying: this needs a configuration fix");
                    return Err(err);
                }
            }
        }

        if !reconnect_enabled {
            return Ok(());
        }

        attempt += 1;
        if max_reconnects > 0 && attempt > max_reconnects {
            anyhow::bail!("giving up after {attempt} connection attempts");
        }

        let delay = backoff_delay(attempt);
        println!(
            "[executor] Reconnecting in {:.1}s (attempt {}) — is the bridge running?",
            delay.as_secs_f32(),
            attempt
        );
        std::thread::sleep(delay);
    }
}

fn is_fatal(err: &anyhow::Error) -> bool {
    let message = format!("{err:#}");
    message.contains("token") || message.contains("protocol")
}

fn backoff_delay(attempt: u32) -> Duration {
    let seconds = 1u64 << attempt.min(5);
    Duration::from_secs(seconds.min(30))
}

fn env_bool(name: &str, default: bool) -> bool {
    env::var(name)
        .map(|value| matches!(value.to_lowercase().as_str(), "1" | "true" | "yes" | "on"))
        .unwrap_or(default)
}

fn run_session(
    server_addr: &str,
    controller: &Controller,
    token: Option<&str>,
) -> Result<SessionEnd> {
    let stream = TcpStream::connect(server_addr)
        .with_context(|| format!("Failed to connect to bridge at {server_addr}"))?;
    stream.set_read_timeout(Some(Duration::from_secs(120)))?;
    stream.set_write_timeout(Some(Duration::from_secs(30)))?;

    let mut writer = stream.try_clone().context("clone tcp stream")?;
    let mut reader = BufReader::new(stream);

    let mut hello = Envelope::new("hello");
    hello.role = Some("executor".into());
    hello.version = Some(PROTOCOL_VERSION.into());
    hello.token = token.map(|value| value.to_string());
    write_line(&mut writer, &hello)?;

    let mut line = String::new();
    reader.read_line(&mut line)?;
    let ack: Envelope =
        serde_json::from_str(line.trim()).context("invalid hello_ack from bridge")?;

    match ack.kind.as_str() {
        "hello_ack" => {}
        "hello_nack" => {
            anyhow::bail!(
                "bridge rejected this executor: {}",
                ack.error.unwrap_or_else(|| "unspecified".into())
            );
        }
        other => anyhow::bail!("expected hello_ack, got {other}"),
    }

    let version_note = ack
        .version
        .as_deref()
        .map(|version| format!(" (protocol {version})"))
        .unwrap_or_default();
    println!("[executor] Connected to bridge{version_note} — waiting for commands");

    loop {
        line.clear();
        match reader.read_line(&mut line) {
            Ok(0) => return Ok(SessionEnd::Disconnected),
            Ok(_) => {}
            Err(e) => {
                // Timeouts are expected while idle; keep waiting.
                if e.kind() == std::io::ErrorKind::WouldBlock
                    || e.kind() == std::io::ErrorKind::TimedOut
                {
                    continue;
                }
                return Err(e.into());
            }
        }

        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }

        let env: Envelope = match serde_json::from_str(trimmed) {
            Ok(value) => value,
            Err(e) => {
                eprintln!("[executor] bad envelope: {e} ({trimmed})");
                continue;
            }
        };

        match env.kind.as_str() {
            "ping" => {
                let mut pong = Envelope::new("pong");
                pong.id = env.id;
                write_line(&mut writer, &pong)?;
            }
            "command" => {
                let shutdown = handle_command(&mut writer, controller, env);
                if shutdown {
                    return Ok(SessionEnd::Requested);
                }
            }
            _ => {}
        }
    }
}

/// Executes one command and returns true when the bridge asked us to shut down.
fn handle_command(writer: &mut TcpStream, controller: &Controller, env: Envelope) -> bool {
    let id = env.id.clone().unwrap_or_default();
    let cmd_value = env.command.unwrap_or(Value::Null);

    let response = match serde_json::from_value::<IPCCommand>(cmd_value) {
        Ok(cmd) => {
            if let Err(e) = cmd.validate() {
                crate::ipc_handler::IPCResponse::failure(e.to_string())
            } else {
                IPCHandler::execute_command(controller, cmd)
            }
        }
        Err(e) => crate::ipc_handler::IPCResponse::failure(format!("invalid command: {e}")),
    };

    let shutdown = response
        .data
        .as_ref()
        .and_then(|data| data.get("shutdown"))
        .and_then(|value| value.as_bool())
        .unwrap_or(false);

    let mut result = Envelope::new("result");
    result.id = Some(id);
    result.success = Some(response.success);
    result.data = response.data;
    result.error = response.error;

    if let Err(e) = write_line(writer, &result) {
        eprintln!("[executor] failed to send result: {e}");
    }

    shutdown
}

fn write_line(writer: &mut TcpStream, env: &Envelope) -> Result<()> {
    let mut bytes = serde_json::to_vec(env)?;
    bytes.push(b'\n');
    writer.write_all(&bytes)?;
    writer.flush()?;
    Ok(())
}
