use serde::{Deserialize, Serialize};
use anyhow::Result;
use std::fs;

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Features {
    pub ci_api: bool,
    pub wizards: bool,
    pub history_days: u32,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Entitlements {
    pub edition: String,
    pub max_hosts: Option<usize>,
    pub demo_stacks: Option<usize>,
    pub features: Features,
    pub org: Option<String>,
}

impl Default for Entitlements {
    fn default() -> Self {
        Self {
            edition: "Community".to_string(),
            max_hosts: Some(15),
            demo_stacks: Some(3),
            features: Features { ci_api: true, wizards: true, history_days: 30 },
            org: None,
        }
    }
}

impl Entitlements {
    pub fn load() -> Result<Self> {
        let env_key = std::env::var("DDUI_LICENSE").ok();
        if let Some(json) = env_key {
            if let Ok(e) = serde_json::from_str::<Entitlements>(&json) {
                return Ok(e);
            }
        }
        let path = std::env::var("DDUI_LICENSE_PATH").unwrap_or_else(|_| "/run/secrets/ddui_license".into());
        if let Ok(buf) = fs::read_to_string(path) {
            if let Ok(e) = serde_json::from_str::<Entitlements>(&buf) {
                return Ok(e);
            }
        }
        Ok(Self::default())
    }
}
