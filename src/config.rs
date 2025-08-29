use anyhow::{anyhow, Result};

#[derive(Clone)]
pub struct AppConfig {
    pub bind_addr: String,
    pub scan_kind: String,
    pub scan_root: String,
    pub refresh_interval: String,
    pub license_env: String,
    pub license_path: String,
}

impl AppConfig {
    pub fn from_env() -> Result<Self> {
        let bind_addr = std::env::var("DDUI_BIND").unwrap_or_else(|_| "0.0.0.0:3000".to_string());
        let scan_kind = std::env::var("DDUI_SCAN_KIND").unwrap_or_else(|_| "local".to_string());
        let scan_root = std::env::var("DDUI_SCAN_ROOT").unwrap_or_else(|_| "/opt/docker/ant-parade/docker-compose".to_string());
        let refresh_interval = std::env::var("DDUI_REFRESH_INTERVAL").unwrap_or_else(|_| "10m".to_string());
        let license_env = std::env::var("DDUI_LICENSE_ENV").unwrap_or_else(|_| "DDUI_LICENSE".to_string());
        let license_path = std::env::var("DDUI_LICENSE_PATH").unwrap_or_else(|_| "/run/secrets/ddui_license".to_string());

        if scan_kind != "local" && scan_kind != "repo" {
            return Err(anyhow!("DDUI_SCAN_KIND must be 'local' or 'repo'"));
        }

        Ok(Self {
            bind_addr,
            scan_kind,
            scan_root,
            refresh_interval,
            license_env,
            license_path,
        })
    }
}
