use std::sync::Arc;
use axum::extract::FromRef;

use crate::{config::AppConfig, entitlements::Entitlements};

#[derive(Clone)]
pub struct AppState {
    pub cfg: AppConfig,
    pub ents: Entitlements,
}

impl AppState {
    pub fn new(cfg: AppConfig, ents: Entitlements) -> Self { Self { cfg, ents } }
}

impl FromRef<AppState> for AppConfig {
    fn from_ref(s: &AppState) -> AppConfig { s.cfg.clone() }
}
impl FromRef<AppState> for Entitlements {
    fn from_ref(s: &AppState) -> Entitlements { s.ents.clone() }
}

pub mod health;
pub mod inventory;
pub mod ci_run;
