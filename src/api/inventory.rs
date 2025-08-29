use axum::{Json, extract::State};
use serde::{Serialize, Deserialize};
use std::fs;
use std::path::{Path, PathBuf};
use crate::config::AppConfig;

#[derive(Serialize, Deserialize, Clone)]
pub struct Container {
    pub name: String,
    pub image: String,
    pub state: String,
}

#[derive(Serialize, Deserialize, Clone)]
pub struct Stack {
    pub name: String,
    pub r#type: String,
    pub path: String,
    pub sops: bool,
    pub containers: Vec<Container>,
}

#[derive(Serialize, Deserialize, Clone)]
pub struct Host {
    pub host: String,
    pub groups: Vec<String>,
    pub stacks: Vec<Stack>,
}

#[derive(Serialize)]
pub struct Inventory { pub hosts: Vec<Host> }

pub async fn get_inventory(State(cfg): State<AppConfig>) -> Json<Inventory> {
    let mut hosts: Vec<Host> = Vec::new();
    let root = PathBuf::from(&cfg.scan_root);
    if root.exists() {
        if let Ok(host_dirs) = fs::read_dir(&root) {
            for host_entry in host_dirs.flatten() {
                if !host_entry.path().is_dir() { continue; }
                let host_name = host_entry.file_name().to_string_lossy().to_string();
                let mut stacks: Vec<Stack> = Vec::new();
                if let Ok(stack_dirs) = fs::read_dir(host_entry.path()) {
                    for stack_entry in stack_dirs.flatten() {
                        if !stack_entry.path().is_dir() { continue; }
                        let stack_name = stack_entry.file_name().to_string_lossy().to_string();
                        let dc_yaml = stack_entry.path().join("docker-compose.yaml");
                        let dc_tpl = stack_entry.path().join("docker-compose.tpl.yaml");
                        let rtype = if dc_yaml.exists() || dc_tpl.exists() { "compose" } else { "script" }.to_string();
                        let sops = glob_has_sops(&stack_entry.path());
                        stacks.push(Stack {
                            name: stack_name,
                            r#type: rtype,
                            path: stack_entry.path().to_string_lossy().to_string(),
                            sops,
                            containers: vec![],
                        });
                    }
                }
                hosts.push(Host { host: host_name, groups: vec![], stacks });
            }
        }
    }
    Json(Inventory { hosts })
}

fn glob_has_sops(dir: &Path) -> bool {
    if let Ok(entries) = fs::read_dir(dir) {
        for e in entries.flatten() {
            if let Ok(md) = e.metadata() {
                if md.is_file() {
                    let name = e.file_name().to_string_lossy().to_lowercase();
                    if name.ends_with(".env.sops") || name.contains(".sops.") {
                        return true;
                    }
                }
            }
        }
    }
    false
}
