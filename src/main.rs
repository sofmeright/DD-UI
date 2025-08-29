mod config;
mod entitlements;
mod api;

use axum::{routing::{get, post}, Router};
use tower_http::{trace::TraceLayer, cors::CorsLayer};
use std::net::SocketAddr;
use tracing_subscriber::{EnvFilter, fmt};
use tokio::net::TcpListener;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    dotenvy::dotenv().ok();
    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));
    fmt().with_env_filter(filter).compact().init();

    let cfg = config::AppConfig::from_env()?;
    let ents = entitlements::Entitlements::load()?;
    let shared = api::AppState::new(cfg.clone(), ents.clone());

    let app = Router::new()
        .route("/api/healthz", get(api::health::healthz))
        .route("/api/inventory", get(api::inventory::get_inventory))
        .route("/api/ci/run", post(api::ci_run::ci_run))
        .with_state(shared)
        .layer(CorsLayer::permissive())
        .layer(TraceLayer::new_for_http());

    let addr: SocketAddr = cfg.bind_addr.parse()?;
    tracing::info!(%addr, "DDUI starting");
    let listener = TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;
    Ok(())
}