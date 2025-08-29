// src/api/ci_run.rs
use axum::{extract::State, Json, response::IntoResponse};
use serde::Deserialize;
use futures_util::stream::{self};
use axum::body::Body;
use std::time::Duration;
use time::OffsetDateTime;
use tokio_stream::StreamExt;

use crate::{config::AppConfig, entitlements::Entitlements};

#[derive(Deserialize)]
pub struct CiRunRequest {
    pub mode: String,
}

pub async fn ci_run(
    State(_cfg): State<AppConfig>,
    State(ents): State<Entitlements>,
    Json(_req): Json<CiRunRequest>,
) -> impl IntoResponse {
    let start = OffsetDateTime::now_utc().format(&time::format_description::well_known::Rfc3339).unwrap_or_else(|_| "now".into());
    let lines = vec![
        serde_json::json!({"ts": start, "level": "info", "msg": "run started", "edition": ents.edition}),
        serde_json::json!({"level": "info", "msg": "planning"}),
        serde_json::json!({"level": "info", "msg": "nothing to change"}),
        serde_json::json!({"level": "done", "summary": {"hosts": 0, "stacks": 0, "changed": 0, "failed": 0}}),
    ];
    let stream = stream::iter(lines.into_iter())
        .throttle(Duration::from_millis(200))
        .map(|v| Ok::<_, std::io::Error>(format!("{}\n", serde_json::to_string(&v).unwrap())));
    axum::http::Response::builder()
        .header("Content-Type", "application/x-ndjson")
        .body(Body::from_stream(stream))
        .unwrap()
}