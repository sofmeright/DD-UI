use axum::{Json, extract::State};
use serde::Serialize;

use crate::entitlements::Entitlements;

#[derive(Serialize)]
struct Health { status: &'static str, edition: String }

pub async fn healthz(State(ents): State<Entitlements>) -> Json<Health> {
    Json(Health { status: "ok", edition: ents.edition })
}
