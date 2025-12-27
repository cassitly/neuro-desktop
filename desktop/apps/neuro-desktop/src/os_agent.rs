use std::process::{Command, Stdio};
use std::io::{Write, BufRead, BufReader};

use serde::Serialize;

pub struct OsAgent {
    stdin: std::process::ChildStdin,
    stdout: BufReader<std::process::ChildStdout>,
}

#[derive(Serialize)]
struct CommandMsg<'a> {
    action: &'a str,
    x: Option<i32>,
    y: Option<i32>,
    text: Option<&'a str>,
}

use rust_core::paths::{bundled_python, bundled_packages};

impl OsAgent {
    pub fn start() -> Self {
        let python = bundled_python();
        let python_home = bundled_packages();
        println!("Python executable: {}", python.display());
        println!("Python home: {}", python_home.display());

        let python_lib = python_home.join("python").join("Lib");
        let python_site_packages = python_lib.join("site-packages");
        
        println!("Python Lib: {}", python_lib.display());

        let mut child = Command::new(&python)
            .arg(python_home.join("python").join("os-driver"))
            // PYTHONHOME should point to where Lib is, not to binary
            .env("PYTHONHOME", python_home.join("python"))
            .env("PYTHONNOUSERSITE", "1")
            .env("PYTHONDONTWRITEBYTECODE", "1")
            // Include both Lib and site-packages in PYTHONPATH
            .env("PYTHONPATH", format!(
                "{};{}",
                python_lib.display(),
                python_site_packages.display()
            ))
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::inherit())
            .spawn()
            .expect("Failed to start OS agent");

        let stdin = child.stdin.take().unwrap();
        let stdout = BufReader::new(child.stdout.take().unwrap());

        Self { stdin, stdout }
    }

    fn send(&mut self, msg: &impl Serialize) {
        let json = serde_json::to_string(msg).unwrap();
        writeln!(self.stdin, "{}", json).unwrap();

        let mut response = String::new();
        self.stdout.read_line(&mut response).unwrap();

        if !response.contains("\"ok\"") {
            panic!("OS agent error: {}", response);
        }
    }

    pub fn move_mouse(&mut self, x: i32, y: i32) {
        self.send(&CommandMsg {
            action: "move_mouse",
            x: Some(x),
            y: Some(y),
            text: None,
        });
    }

    pub fn click(&mut self, x: i32, y: i32) {
        self.send(&CommandMsg {
            action: "click",
            x: Some(x),
            y: Some(y),
            text: None,
        });
    }

    pub fn type_text(&mut self, text: &str) {
        self.send(&CommandMsg {
            action: "type",
            x: None,
            y: None,
            text: Some(text),
        });
    }
}
