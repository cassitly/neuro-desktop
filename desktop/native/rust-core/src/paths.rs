use std::path::PathBuf;
use std::env;

pub fn bundled_python() -> PathBuf {
    let exe = env::current_exe().unwrap();
    let root = exe.parent().unwrap();

    root.join("python")
        .join("binary")
        .join("python.exe")
}

pub fn bundled_packages() -> PathBuf {
    let exe = env::current_exe().unwrap();
    let root = exe.parent().unwrap();

    root.to_path_buf()
}