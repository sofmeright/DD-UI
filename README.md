# Designated Driver UI (DDUI)
> Declarative, security-first Docker orchestration. DDUI compares runtime state (containers on your hosts) to declared state (your IaC repo), shows drift, and puts encryption (SOPS/AGE) and DevOps ergonomics first.

## Status

### Pre-release / Work in progress
> You’re welcome to run it locally and preview alongside development. DDUI is under active development, but there are gaps and rough edges. Expect breaking changes and incomplete features.

### Maintainer Note
I’m currently dealing with financial hardship and can’t reliably keep my 5-node homelab online (it runs ~$300–$500/mo). I’m also between roles and may have periods without power/utilities, so progress could pause at times. I’m still committed to the project—just not always able to work on it full-time. Thanks for your understanding and any help you can offer.

### What DDUI does today
- Inventory: list hosts; drill into a host to see stacks/containers.
- Sync: one click triggers:
- IaC scan (local repo) and
- runtime scan per host (Docker).
- Compare: show runtime vs desired (images, services); per-stack drift indicator.
- Per-host search, fixed table layout, ports rendered one mapping per line.
- SOPS awareness: detect encrypted files; don’t decrypt by default.
- Auth: OIDC (e.g., Zitadel/Okta/Auth0). Session probe, login, and logout (RP-logout optional).
- API: /api/... (JSON), static SPA served by backend.

### Planned / WIP highlights
- Health-aware state pills (running/healthy/exited etc.) with Portainer-style visuals.
- Stack Files page: view (and optionally edit) compose/env/scripts vs runtime context; gated decryption for SOPS.
- Safer change workflows (preview/validate/apply).
- Additional inventory backends.

Features are evolving fast; please treat all APIs and UI as unstable.

## Architecture (high level)
- Backend (Go):
    - OIDC auth, sessions (cookie).
    - Scans: Docker hosts (runtime) + IaC repo (local).
    - Postgres for persistence (migrations in src/api/migrations).
    - Serves the SPA.
- Frontend (Vite/React + Tailwind/shadcn):
    - Hosts page (metrics + search + Sync).
    - Host detail (stacks, drift, per-host search).

## Requirements
- Docker reachable from the DDUI backend to each host you list (TCP or local socket).
- PostgreSQL 14+
- Node 18+ (for dev UI), Go 1.21+ (backend)
- OIDC provider (Tested with Zitadel) or run in “local only” with /api/session returning no user (login page will redirect).

Rust-based orchestrator skeleton with:
- `GET /api/healthz`
- `GET /api/inventory` (scans local `DDUI_SCAN_ROOT`)
- `POST /api/ci/run` (streams NDJSON stub)

## Quick start (developer mode)
> Best for hacking on the UI/API locally.
1. Postgres
```bash
docker run -d --name ddui-pg -p 5432:5432 \
  -e POSTGRES_PASSWORD=devpass -e POSTGRES_USER=ddui -e POSTGRES_DB=ddui \
  postgres:15
```
Set DATABASE_URL for the backend:
```bash
export DATABASE_URL=postgres://ddui:devpass@localhost:5432/ddui?sslmode=disable
```
2. OIDC (Zitadel example)
Create an OAuth 2.0 Web client:
- Redirect URL: https://your-ddui.example.com/auth/callback (or http://localhost:8080/auth/callback for dev)
- (Optional) Post-logout redirect: http://localhost:8080/
- Scopes: openid email profile
Environment (dev):
```bash
export OIDC_ISSUER_URL="https://<your-zitadel-domain>/.well-known/openid-configuration"
export OIDC_CLIENT_ID="<client-id>"
export OIDC_CLIENT_SECRET="<client-secret>"    # supports "@/path/to/secret"
export OIDC_REDIRECT_URL="http://localhost:8080/auth/callback"
# Optional hardening / ergonomics
export OIDC_SCOPES="openid email profile"
export OIDC_ALLOWED_EMAIL_DOMAIN=""            # e.g. "example.com" to restrict
export COOKIE_DOMAIN=""                         # e.g. ".example.com" in prod
# If unset, DDUI infers COOKIE_SECURE from the redirect URL scheme
# export COOKIE_SECURE=true|false
```
3. Point DDUI at your IaC repo (local)
Mount or place your repo under a root (default /data) with this layout:
```bash
/data/
  docker-compose/
    <scope-name>/
      <stack-name>/
        compose.yaml|docker-compose.yaml
        .env / *.env / *_secret.env (SOPS detection supported)
        pre.sh / deploy.sh / post.sh (optional)
```
- <scope-name> is either a host name or a group name.
- DDUI auto-detects if a scope matches a host in your inventory; otherwise it’s treated as a group.
Env (if you customize):
```bash
export DDUI_IAC_ROOT="/data"
export DDUI_IAC_DIRNAME="docker-compose"
# Decryption is OFF by default; enable explicit opt-in flows later:
# export DDUI_SOPS_DECRYPT_ENABLE=1
```
4. Run backend
```bash
cd src/api
go run .
# or: go build -o ddui && ./ddui
```
The backend will run DB migrations automatically at startup (ensure DATABASE_URL is set).
5. Run frontend
```bash
cd ui
pnpm install
pnpm dev
```
Visit http://localhost:5173
 (or the port Vite prints).
In production, the Go server serves the built UI; during dev it’s fine to run separately.

## Quick start (docker-compose)
This brings up Postgres and the backend; you’ll build the UI and point the backend at it, or use the dev server.
```yaml
version: "3.8"
services:
  
  ddui-postgres:
    container_name: ddui-postgres
    image: postgres:16-alpine
    environment:
      - POSTGRES_DB=ddui
      - POSTGRES_USER=prplanit
      - POSTGRES_PASSWORD_FILE=/run/secrets/pg_pass
    volumes:
      - /opt/docker/ddui/postgres:/var/lib/postgresql/data
    secrets:
      - pg_pass
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $POSTGRES_USER -d $POSTGRES_DB"]
      interval: 5s
      timeout: 3s
      retries: 20
      
  ddui-app:
    container_name: ddui-app
    depends_on:
      ddui-postgres:
        condition: service_healthy
    image: prplanit/ddui:v0.1.6
    ports:
      - "3000:8080"
    env_file: stack.env
    environment:
      - DDUI_BIND=0.0.0.0:8080
      - DDUI_DB_HOST=ddui-postgres
      - DDUI_DB_PORT=5432
      - DDUI_DB_NAME=ddui
      - DDUI_DB_USER=prplanit
      - DDUI_DB_PASS_FILE=/run/secrets/pg_pass
      - DDUI_DB_SSLMODE=disable
      - DDUI_DB_MIGRATE=true
      # or provide a single DSN:
      # - DDUI_DB_DSN=postgres://ddui:...@db:5432/ddui?sslmode=disable
      # - DDUI_INVENTORY_PATH=/data/inventory
      - DDUI_SCAN_KIND=local
      - DDUI_SCAN_ROOT=/data/docker-compose
      # SSH to each host for docker scans
      - DDUI_SSH_USER=kai           # or a limited user in docker group
      - DDUI_SSH_PORT=22
      - DDUI_SSH_USE_SUDO=false      # true if your user needs sudo
      - DDUI_SSH_STRICT_HOST_KEY=false
      - DDUI_SSH_KEY=@/run/secrets/ddui_ssh_key
      # Scanner Settings
      - DDUI_SCAN_AUTO=true
      - DDUI_SCAN_ON_START=true
      - DDUI_SCAN_INTERVAL=1m
      - DDUI_SCAN_HOST_TIMEOUT=45s
      - DDUI_SCAN_CONCURRENCY=3
      # Remote & SSH Settings
      - DDUI_LOCAL_HOST=anchorage
      #- COOKIE_DOMAIN=anchorage
      - COOKIE_SECURE=false
      - OIDC_ISSUER_URL=https://sso.prplanit.com
      - OIDC_REDIRECT_URL=http://anchorage:3000/auth/callback
      - OIDC_SCOPES=openid email profile
      # - OIDC_ALLOWED_EMAIL_DOMAIN # (optional; blocks others)
    secrets:
      - pg_pass
    volumes:
      - /opt/docker/ddui/data:/data
      - /var/run/docker.sock:/var/run/docker.sock

secrets:
  ddui_ssh_key:
    file: /opt/docker/ddui/secrets/id_ed25519   # your private key
  pg_pass:
    file: /opt/docker/ddui/secrets/postgres_password.txt
```
Build the UI once:
```bash
cd ui && pnpm install && pnpm build
```
Then hit http://localhost:8080.

## Using DDUI
1. Log in (OIDC). You’ll be redirected to /auth/login if no session.
2. Add hosts to inventory. Currently hosts are stored in the DB; the API supports reload from a path if you want to seed via file:
```bash
# POST /api/inventory/reload with an optional { "path": "/data/inventory.yaml" }
curl -sS -X POST -H "Content-Type: application/json" \
  -d '{"path":"/data/inventory.yaml"}' \
  http://localhost:8080/api/inventory/reload
```
inventory.yaml (example) (Ansible formatted inventory, yaml/ini supported)
```yaml
all:
  hosts:
# GPU Accelerated:
    anchorage:
      ansible_host: 10.30.1.122
    leaf-cutter:
      ansible_host: 10.13.37.141
```
3. Click Sync on the Hosts page (or “Scan” per host). This will:
    - Scan IaC (/data/docker-compose/...), persist stacks/services/files.
    - Scan runtime per host (containers, images, ports, health).
4. Drill into a host to see:
    - Stacks merged from runtime and IaC.
    - For each row: name, state, image (runtime → desired), created, IP, ports (one per line), owner.
    - Per-host search box (filters rows).
5. Metrics: Hosts, Stacks, Containers, Drift, Errors aggregate across filtered hosts.
6. SOPS: encrypted .env files are detected (marked), but not decrypted unless you explicitly enable gated flows (coming UI).

## IaC layout details
- DDUI walks <root>/<dirname>/<scope>/<stack> (defaults /data/docker-compose/*/*).
- It records:
    - compose file (if present),
    - env files (SOPS detection via markers),
    - scripts pre.sh, deploy.sh, post.sh,
    - parsed services (image, labels, ports, volumes, env keys).
- Scopes
    - If <scope> equals a known host, it’s a host scope.
    - Otherwise it’s a group scope (applies to any host in that group).
- Drift
    - Different image than desired, a missing desired container/service, or IaC with no runtime => drift.

## Security posture (Community)
- Encryption aware: SOPS/AGE detection; decrypt-to-tmpfs design (off by default).
- Least privilege: no secrets are persisted by default.
- Redacted logs planned where sensitive values are involved.
- OIDC cookie security: COOKIE_SECURE inferred from OIDC_REDIRECT_URL unless you override; SameSite=Lax.

## Troubleshooting
**Logout doesn’t seem to work**
- The backend expects POST /logout (or /auth/logout) and then redirects.
    - Use a <form method="post" action="/logout"> in the UI (already wired).
- Ensure cookie settings are correct:
    - Domain: set COOKIE_DOMAIN to your parent domain (e.g. .example.com) if the app is on a subdomain.
    - Secure: if you serve over HTTPS, cookies must be Secure (COOKIE_SECURE=true or let DDUI infer it from an https:// redirect URL).
    - If behind a proxy, make sure X-Forwarded-Proto is set so the app knows it’s HTTPS.
- (Optional) RP-Initiated Logout: if your OP supports end_session_endpoint, you can extend the handler to redirect there with id_token_hint and post_logout_redirect_uri. DDUI already stores id_token for this purpose.
**SQL error**: there is no unique or exclusion constraint matching the ON CONFLICT specification (SQLSTATE 42P10)
- Make sure your migrations are up to date. For iac_repos we upsert on (kind, root_path) now (not just root_path). Re-apply 010_iac.sql or run the fixed migration and restart the backend.
**Can’t log in**
- Verify OIDC client Redirect URL matches your DDUI base URL + /auth/callback.
- Check your reverse proxy headers (host/proto) so the callback builds the right URL.
- Inspect backend logs for auth: messages.

## Environment Variables
| Variable                                | Default                | Description                                                   |
| --------------------------------------- | ---------------------- | ------------------------------------------------------------- |
| `DATABASE_URL`                          | —                      | Postgres connection string                                    |
| `OIDC_ISSUER_URL`                       | —                      | Provider discovery URL (`…/.well-known/openid-configuration`) |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | —                      | OAuth client                                                  |
| `OIDC_REDIRECT_URL`                     | —                      | e.g. `http://localhost:8080/auth/callback`                    |
| `OIDC_SCOPES`                           | `openid email profile` | Space-separated scopes                                        |
| `OIDC_ALLOWED_EMAIL_DOMAIN`             | empty                  | Restrict logins to a domain                                   |
| `COOKIE_DOMAIN`                         | empty                  | e.g. `.example.com`                                           |
| `COOKIE_SECURE`                         | inferred               | `true/false` (if unset, inferred from redirect URL scheme)    |
| `DDUI_UI_DIR`                           | `/home/ddui/ui/dist`   | Where built SPA is served from                                |
| `DDUI_IAC_ROOT`                         | `/data`                | IaC repository root                                           |
| `DDUI_IAC_DIRNAME`                      | `docker-compose`       | Folder inside root DDUI scans                                 |
| `DDUI_SOPS_DECRYPT_ENABLE`              | unset                  | (Future) enable gated decrypt actions                         |

## Contributing
- File issues with steps, logs, and versions.
- Small, focused PRs are best (typos, error handling, UI polish).
- Sample IaC directories welcome!

## Support / Sponsorship
If you’d like to help keep the project moving:

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/T6T41IT163)

## License
I want to be FOSS/Open as possible, but with financial tragedy imminent and the tech industry not seeing me as capable of even doing entry level work... I may have to structure this as open core with validation of use being home/commercial. I will always love the Open Source Community, and you will always be my priority but I want to survive and justify my hours so I can make useful products that can further our scene! If I see any sort of sponsorship or find stable employment this will likely be GNU or Apache license.
