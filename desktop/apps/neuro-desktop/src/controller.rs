use anyhow::Result;
use pyo3::prelude::*;
use pyo3::types::PyTuple;
use serde_json::Value;
use std::env;

pub struct Controller {
    monitor: Py<PyAny>,
    mouse: Py<PyAny>,
    keyboard: Py<PyAny>,
    parser: Py<PyAny>,
}

impl Controller {
    pub fn initialize_drivers() -> Result<Self> {
        Python::with_gil(|py| -> PyResult<Self> {
            // -------------------------------------------------
            // Configure Python path
            // -------------------------------------------------
            let sys = py.import_bound("sys")?;
            let path = sys.getattr("path")?;
            let path = path.downcast::<pyo3::types::PyList>()?;

            let add_path = |candidate: std::path::PathBuf| -> PyResult<()> {
                if candidate.exists() {
                    path.insert(0, candidate.to_string_lossy().as_ref())?;
                }
                Ok(())
            };

            // Bundled Python runtime lives next to the executable. This used to
            // come from a separate helper crate; a five-line path join is not
            // worth a second crate and a second build step
            // (see docs/ARCHITECTURE.md).
            let runtime_python_root = env::current_exe()
                .ok()
                .and_then(|exe| exe.parent().map(|dir| dir.join("python")))
                .unwrap_or_else(|| std::path::PathBuf::from("python"));
            add_path(runtime_python_root.clone())?;
            add_path(runtime_python_root.join("Lib"))?;
            add_path(runtime_python_root.join("Lib").join("site-packages"))?;

            // Development fallback when running from source without a bundled runtime.
            if !runtime_python_root.exists() {
                if let Ok(exe_path) = env::current_exe() {
                    if let Some(exe_dir) = exe_path.parent() {
                        let mut probe = exe_dir.to_path_buf();
                        for _ in 0..6 {
                            let dev_python_root = probe.join("backend").join("python");
                            if dev_python_root.exists() {
                                add_path(dev_python_root.clone())?;
                                add_path(dev_python_root.join("Lib"))?;
                                add_path(dev_python_root.join("Lib").join("site-packages"))?;
                                break;
                            }
                            if !probe.pop() {
                                break;
                            }
                        }
                    }
                }
            }

            // -------------------------------------------------
            // Import lib module (the control driver's entrypoint)
            // -------------------------------------------------
            let lib = py.import_bound("controller.lib")?;

            // -------------------------------------------------
            // Call factory function
            // -------------------------------------------------
            let result = lib.getattr("initialize_driver")?.call0()?;
            let tuple = result.downcast::<PyTuple>()?;

            Ok(Self {
                monitor: tuple.get_item(0)?.into(),
                mouse: tuple.get_item(1)?.into(),
                keyboard: tuple.get_item(2)?.into(),
                parser: tuple.get_item(3)?.into(),
            })
        })
        .map_err(Into::into)
    }

    // =====================================================
    // Script execution (preferred API)
    // =====================================================

    pub fn run_script(&self, script: &str) -> Result<()> {
        Python::with_gil(|py| {
            self.parser.bind(py).getattr("parse")?.call1((script,))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    // Used to execute manual low-level calls (required when calling low-level APIs)
    pub fn execute_instructions(&self) -> Result<()> {
        Python::with_gil(|py| {
            self.keyboard.bind(py).getattr("execute")?.call0()?;
            self.mouse.bind(py).getattr("execute")?.call0()?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    // =====================================================
    // Low-level direct calls (optional)
    // =====================================================

    pub fn mouse_move(&self, x: i32, y: i32) -> Result<()> {
        Python::with_gil(|py| {
            self.mouse.bind(py).getattr("queue_move")?.call1((x, y))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    pub fn mouse_click(&self, button: &str) -> Result<()> {
        Python::with_gil(|py| {
            self.mouse
                .bind(py)
                .getattr("queue_click")?
                .call1((button,))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    pub fn type_text(&self, text: &str) -> Result<()> {
        Python::with_gil(|py| {
            self.keyboard.bind(py).getattr("type")?.call1((text,))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    /// True when the controlled machine has no display session (or the operator
    /// forced headless mode). Reported by get_status so the bridge, the dashboard
    /// and Neuro can tell why input actions are unavailable.
    pub fn is_headless(&self) -> bool {
        Python::with_gil(|py| {
            py.import_bound("controller.gui_stub")
                .and_then(|module| module.getattr("is_headless")?.call0())
                .and_then(|value| value.extract::<bool>())
                .unwrap_or(false)
        })
    }

    /// Run one command line through the Python shell capability.
    ///
    /// The Python side enforces the allowlist/denylist firewall again and
    /// returns a transcript; both a refusal and a non-zero exit come back as
    /// text, because the model needs to read the exit status.
    pub fn run_shell(
        &self,
        command: &str,
        cwd: Option<&str>,
        timeout: Option<f64>,
    ) -> Result<String> {
        Python::with_gil(|py| {
            let lib = py.import_bound("controller.lib")?;
            let result = lib
                .getattr("run_shell")?
                .call1((command, cwd, timeout))?;
            Ok::<String, PyErr>(result.extract::<String>()?)
        })
        .map_err(Into::into)
    }

    pub fn clear_action_queue(&self) -> Result<()> {
        Python::with_gil(|py| {
            self.mouse.bind(py).getattr("clear")?.call0()?;
            self.keyboard.bind(py).getattr("clear")?.call0()?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    /// Press several keys together.
    ///
    /// The previous implementation passed one whitespace-joined string to the
    /// variadic Python `shortcut(*keys)`, which iterated over single characters
    /// ("ctrl s" became c, t, r, l, space, s). Keys are now passed as a real
    /// tuple, and the single-string entry point joins them explicitly.
    pub fn keyboard_shortcut(&self, keys: &[String]) -> Result<()> {
        self.key_combo(keys)
    }

    pub fn key_combo(&self, keys: &[String]) -> Result<()> {
        Python::with_gil(|py| {
            let args = PyTuple::new_bound(py, keys.iter());
            self.keyboard.bind(py).getattr("combo")?.call1(args)?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    /// Hold a key for a bounded time (game movement).
    pub fn key_hold_for(&self, key: &str, seconds: f64) -> Result<()> {
        Python::with_gil(|py| {
            self.keyboard
                .bind(py)
                .getattr("hold_for")?
                .call1((key, seconds))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    /// Release every key and mouse button the executor is holding.
    pub fn release_all_input(&self) -> Result<()> {
        Python::with_gil(|py| {
            self.keyboard.bind(py).getattr("release_all")?.call0()?;
            self.mouse.bind(py).getattr("release_all")?.call0()?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    /// Relative pointer movement for in-game mouse-look.
    pub fn mouse_move_relative(&self, dx: i32, dy: i32, duration: f64) -> Result<()> {
        Python::with_gil(|py| {
            self.mouse
                .bind(py)
                .getattr("queue_move_rel")?
                .call1((dx, dy, duration))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    /// Hold a mouse button for a bounded time (sustained fire / aim).
    pub fn mouse_hold_for(&self, button: &str, seconds: f64) -> Result<()> {
        Python::with_gil(|py| {
            self.mouse
                .bind(py)
                .getattr("queue_hold")?
                .call1((button, seconds))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    pub fn hold_key_down(&self, key: &str) -> Result<()> {
        Python::with_gil(|py| {
            self.keyboard.bind(py).getattr("hold")?.call1((key,))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    pub fn release_key(&self, key: &str) -> Result<()> {
        Python::with_gil(|py| {
            self.keyboard.bind(py).getattr("release")?.call1((key,))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    pub fn press_key(&self, key: &str) -> Result<()> {
        Python::with_gil(|py| {
            self.keyboard.bind(py).getattr("press")?.call1((key,))?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }

    // =====================================================
    // Telemetry access
    // =====================================================

    pub fn get_current_mouse_position(&self) -> Result<(i32, i32)> {
        Python::with_gil(|py| {
            let result = self
                .monitor
                .bind(py)
                .getattr("get_current_mouse_position")?
                .call0()?;
            let tuple = result.downcast::<PyTuple>()?;
            let x = tuple.get_item(0)?.extract::<i32>()?;
            let y = tuple.get_item(1)?.extract::<i32>()?;
            Ok::<(i32, i32), PyErr>((x, y))
        })
        .map_err(Into::into)
    }

    pub fn action_history(&self) -> Result<String> {
        Python::with_gil(|py| {
            let history = self
                .monitor
                .bind(py)
                .getattr("get_action_history")?
                .call0()?;

            Ok::<_, PyErr>(history.str()?.to_string())
        })
        .map_err(Into::into)
    }

    pub fn action_history_json(&self) -> Result<Value> {
        Python::with_gil(|py| {
            let history = self
                .monitor
                .bind(py)
                .getattr("get_action_history")?
                .call0()?;

            let json = py.import_bound("json")?;
            let history_json = json.getattr("dumps")?.call1((history,))?;
            let history_str = history_json.extract::<String>()?;

            Ok::<Value, PyErr>(serde_json::from_str(&history_str).unwrap_or(Value::Null))
        })
        .map_err(Into::into)
    }

    pub fn get_active_window(&self) -> Result<Option<String>> {
        Python::with_gil(|py| {
            let result = self
                .monitor
                .bind(py)
                .getattr("get_active_window")?
                .call0()?;
            if result.is_none() {
                return Ok(None);
            }
            Ok::<Option<String>, PyErr>(Some(result.extract::<String>()?))
        })
        .map_err(Into::into)
    }

    pub fn get_open_windows(&self) -> Result<Vec<String>> {
        Python::with_gil(|py| {
            let result = self.monitor.bind(py).getattr("get_open_windows")?.call0()?;
            Ok::<Vec<String>, PyErr>(result.extract::<Vec<String>>()?)
        })
        .map_err(Into::into)
    }

    pub fn get_screen_size(&self) -> Result<(i32, i32)> {
        Python::with_gil(|py| {
            let result = self.monitor.bind(py).getattr("get_screen_size")?.call0()?;
            let tuple = result.downcast::<PyTuple>()?;
            let width = tuple.get_item(0)?.extract::<i32>()?;
            let height = tuple.get_item(1)?.extract::<i32>()?;
            Ok::<(i32, i32), PyErr>((width, height))
        })
        .map_err(Into::into)
    }

    pub fn get_running_processes(&self) -> Result<Vec<String>> {
        Python::with_gil(|py| {
            let result = self
                .monitor
                .bind(py)
                .getattr("get_running_processes")?
                .call0()?;
            Ok::<Vec<String>, PyErr>(result.extract::<Vec<String>>()?)
        })
        .map_err(Into::into)
    }

    pub fn capture_screen_to_file(&self, path: &str) -> Result<String> {
        Python::with_gil(|py| {
            let result = self
                .monitor
                .bind(py)
                .getattr("capture_screen_to_file")?
                .call1((path,))?;
            Ok::<String, PyErr>(result.extract::<String>()?)
        })
        .map_err(Into::into)
    }

    pub fn shutdown(&self) -> Result<()> {
        Python::with_gil(|py| {
            self.monitor.bind(py).getattr("shutdown")?.call0()?;
            Ok::<(), PyErr>(())
        })
        .map_err(Into::into)
    }
}

// // Example usage
//
// let controller = Controller::initialize_drivers()?;
//
// // High-level script
// controller.run_script(r#"
// TYPE "git status"
// ENTER
// WAIT 0.3
// TYPE "git commit -m 'fix'"
// ENTER
// "#)?;
//
// // Inspect what happened
// println!("{}", controller.action_history()?);
//
// // Cleanup
// controller.shutdown()?;
