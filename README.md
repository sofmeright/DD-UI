# <img src="ui/public/DDUI-Logo.png" alt="DDUI (Designated Driver UI)" width="35" height="35" /> DDUI (Designated Driver UI)
> Declarative, security-first Docker orchestration. DDUI compares runtime state (containers on your hosts) to declared state (your IaC repo), shows drift, and puts encryption (SOPS/AGE) and DevOps ergonomics first.

<img src="example/screenshots/DDUI-deployments-host.png" width="750" /><img src="example/screenshots/DDUI-deployments-stack.png" width="750" />

---

## Status

### Nearing MVP / Pre-release
> DDUI is approaching MVP. Core flows (inventory, scans, drift views, stack/file tracking, gated SOPS reveal) are usable. That said, there are gaps and rough edges, and breaking changes can still happen.

#### Project scope & cadence
- This project is built and maintained by **one person**. A large portion of the current codebase landed in **~1 week** of focused work.
- Development will continue and is driven by the maintainer’s available time.
- DDUI is **opinionated**—it reflects how I run Docker in my homelab (declarative IaC, secrets via SOPS/AGE, minimal ceremony). If that resonates, you’ll likely feel right at home.

---

## What DDUI does today
- **Inventory**: list hosts; drill into a host to see stacks/containers.
- **Sync**: one click triggers:
  - **IaC scan** (local repo), and
  - **Runtime scan** per host (Docker).
- **Compare**: show runtime vs desired (images, services); per-stack drift indicator.
- **Usability**: per-host search, fixed table layout, ports rendered one mapping per line.
- **SOPS awareness**: detect encrypted files; don’t decrypt by default (explicit, audited reveal flow).
- **Auth**: OIDC (e.g., Zitadel/Okta/Auth0). Session probe, login, and logout (RP-logout optional).
- **API**: `/api/...` (JSON), static SPA served by backend.
- **SOPS CLI integration**: server executes `sops` for encryption/decryption; no plaintext secrets are stored.
- Health-aware state pills (running/healthy/exited etc.).
- Stack Files page: view (and optionally edit) compose/env/scripts vs runtime context; gated decryption for SOPS.

### Planned / WIP highlights

- Safer change workflows (preview/validate/apply).
- Additional inventory backends.
- Bugfixes
- Further Testing
- UI Refreshes

Features are evolving; treat all APIs and UI as unstable for now.

---

## Architecture (high level)
- **Backend (Go)**:
  - OIDC auth, sessions (cookie).
  - Scans: Docker hosts (runtime) + IaC repo (local).
  - Postgres for persistence (migrations in `src/api/migrations`).
  - Serves the SPA.
  - Calls out to the **`sops`** executable on the server for encrypt/decrypt (expects `sops` on `PATH`).
- **Frontend (Vite/React + Tailwind/shadcn)**:
  - Hosts page (metrics + search + Sync).
  - Host detail (stacks, drift, per-host search).
  - “Reveal SOPS” UX sends an explicit confirmation header to the backend.

> **Note (history)**: an earlier doc mentioned a “Rust-based orchestrator skeleton.” The production backend is Go; that blurb captured the original idea but the current code is Go.

---

## Requirements
- Docker reachable from the DDUI backend to each host you list (TCP or local socket).
- PostgreSQL 14+
- Node 18+ (for dev UI), Go 1.21+ (backend)
- OIDC provider (tested with Zitadel) or run in “local only” with `/api/session` returning no user (login page will redirect).
- **SOPS 3.10+** available on the backend host/container (DDUI calls `sops` by name).  
  The provided Docker image installs it to `/usr/local/bin/sops`.

---

## Quick start (developer mode)
> Best for hacking on the UI/API locally.

1) **Postgres**
```bash
docker run -d --name ddui-pg -p 5432:5432 \
  -e POSTGRES_PASSWORD=devpass -e POSTGRES_USER=ddui -e POSTGRES_DB=ddui \
  postgres:15
```
Set `DATABASE_URL` for the backend:
```bash
export DATABASE_URL=postgres://ddui:devpass@localhost:5432/ddui?sslmode=disable
```

2) **OIDC (Zitadel example)**  
Create an OAuth 2.0 Web client:
- Redirect URL: `https://your-ddui.example.com/auth/callback` (or `http://localhost:8080/auth/callback` for dev)
- (Optional) Post-logout redirect: `http://localhost:8080/`
- Scopes: `openid email profile`

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

3) **Point DDUI at your IaC repo (local)**  
Mount or place your repo under a root (default `/data`) with this layout:
```bash
/data/
  docker-compose/
    <scope-name>/
      <stack-name>/
        compose.yaml|docker-compose.yaml
        .env / *.env / *_secret.env   # SOPS detection supported
        pre.sh / deploy.sh / post.sh  # optional
```
- `<scope-name>` is either a host name or a group name.
- DDUI auto-detects if a scope matches a host in your inventory; otherwise it’s treated as a group.

Env (if you customize):
```bash
export DDUI_IAC_ROOT="/data"
export DDUI_IAC_DIRNAME="docker-compose"
# Gated decrypt is OFF by default; see SOPS section below to enable carefully.
# export DDUI_ALLOW_SOPS_DECRYPT=true
```

4) **Run backend**
```bash
cd src/api
go run .
# or: go build -o ddui && ./ddui
```
The backend runs DB migrations automatically at startup (ensure `DATABASE_URL` is set).

5) **Run frontend**
```bash
cd ui
pnpm install
pnpm dev
```
Visit `http://localhost:5173` (or the port Vite prints).  
In production, the Go server serves the built UI; during dev it’s fine to run separately.

---

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
    image: cr.pcfae.com/prplanit/ddui:v0.3.0
    ports:
      - "3000:8080"
    env_file: .env
    environment:
      - DDUI_BIND=0.0.0.0:8080
      - DDUI_DB_HOST=ddui-postgres
      - DDUI_DB_PORT=5432
      - DDUI_DB_NAME=ddui
      - DDUI_DB_USER=prplanit
      - DDUI_DB_PASS_FILE=/run/secrets/pg_pass
      - DDUI_DB_SSLMODE=disable
      - DDUI_DB_MIGRATE=true
      # - UI_ORIGIN=
      # or provide a single DSN:
      # - DDUI_DB_DSN=postgres://ddui:...@db:5432/ddui?sslmode=disable
      # - DDUI_IAC_SCAN_AUTO=false
      # - DDUI_IAC_SCAN_INTERVAL=90s
      - DDUI_DEVOPS_APPLY=false
      - DDUI_ALLOW_SOPS_DECRYPT=true
      - SOPS_AGE_KEY_FILE=/run/secrets/sops_age_key
      # - SOPS_AGE_RECIPIENTS=age1abc... age1def...   # see SOPS section
      # - DDUI_INVENTORY_PATH=/data/inventory
      - DDUI_SCAN_KIND=local
      - DDUI_SCAN_ROOT=/data/docker-compose
      # SSH to each host for docker scans
      - DDUI_SSH_USER=kai           # or a limited user in docker group
      - DDUI_SSH_PORT=22
      - DDUI_SSH_USE_SUDO=false      # true if your user needs sudo
      - DDUI_SSH_STRICT_HOST_KEY=false
      - DDUI_SSH_KEY=/run/secrets/ddui_ssh_key
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
      - ddui_ssh_key
      - sops_age_key
    volumes:
      - /opt/docker/ddui/data:/data
      - /var/run/docker.sock:/var/run/docker.sock

secrets:
  ddui_ssh_key:
    file: /opt/docker/ddui/secrets/id_ed25519   # your private key
  pg_pass:
    file: /opt/docker/ddui/secrets/postgres_password.txt
  sops_age_key:
    file: /opt/docker/ddui/secrets/sops_age_key.txt
```

### `.env` file
```.env
OIDC_CLIENT_ID=
OIDC_CLIENT_SECRET=
# You can generate the Session Secret via DDUI_SESSION_SECRET="$(openssl rand -hex 64)".
DDUI_SESSION_SECRET=
POSTGRES_USER=
POSTGRES_DB=
# Optional: advertise your dev UI origin so CORS allows credentials
# UI_ORIGIN=http://localhost:5173
```

Build the UI once:
```bash
cd ui && pnpm install && pnpm build
```
Then hit `http://localhost:8080` (or `http://localhost:3000` if you used the mapping above).

---

## SOPS / AGE: keys, encrypt, decrypt

DDUI integrates with **SOPS** to keep secrets encrypted at rest in your IaC repo. The backend calls the `sops` CLI; ensure it’s on `PATH` (our image installs it to `/usr/local/bin/sops`).

### Generate AGE key pair (server-side decrypt capability)
On a **secure workstation** or secrets box:
```bash
# Generate a private key; prints the public recipient on stderr
age-keygen -o /opt/docker/ddui/secrets/sops_age_key.txt
# Show (or copy) the public recipient for encryption (starts with "age1")
age-keygen -y /opt/docker/ddui/secrets/sops_age_key.txt
```
Wire it into Compose as a Docker secret (see `sops_age_key` in the compose file above), and point DDUI at it:
```
SOPS_AGE_KEY_FILE=/run/secrets/sops_age_key
```
> **Never** commit the private key to Git. Treat `/opt/docker/ddui/secrets/sops_age_key.txt` like any other production secret.

### Choose encrypt recipients
To encrypt, SOPS needs one or more **AGE recipients** (public keys). You have two main options:

1. **Environment variable (no repo config required)**  
   Set `SOPS_AGE_RECIPIENTS` with one or more recipients (space-separated):
   ```
   SOPS_AGE_RECIPIENTS="age1teamUser1... age1teamUser2... age1ciKey..."
   ```
   DDUI will pass each recipient to `sops` as `--age <recipient>` during encryption.

2. **`.sops.yaml` in your repo**  
   Store creation rules in the repo so `sops` knows what to use per path:
   ```yaml
   # /data/.sops.yaml
   creation_rules:
     - path_regex: 'docker-compose/.+/.+/.+\.env$'
       encrypted_regex: '^(SECRET_|PASSWORD_|API_KEY|TOKEN)'
       key_groups:
         - age:
             - age1teamUser1...
             - age1ciKey...
       # (Optional) tell sops the input format for .env files
       # (DDUI already hints this when encrypting *.env)
       # unencrypted_suffix: _unencrypted
   ```

> If you see `sops: encrypt failed: ... config file not found, or has no creation rules, and no keys provided ...`, it means neither `SOPS_AGE_RECIPIENTS` nor a `.sops.yaml` with matching creation rules were found. Provide recipients or add a config.

### Encrypting files
- **From the DDUI UI**: creating/updating a file with the **SOPS** toggle ON (or naming it `*_private.env` / `*_secret.env`) will attempt to run:
  - `sops -e -i [--input-type dotenv] <file>`
  - Plus `--age <recipient>` for each recipient present in `SOPS_AGE_RECIPIENTS`.
- **From CLI** (local dev):
  ```bash
  # .env files (dotenv-aware):
  sops --input-type dotenv --age age1recipient... -e -i /data/docker-compose/host/stack/app.env
  # generic YAML/JSON/TOML:
  sops --age age1recipient... -e -i /data/docker-compose/host/stack/compose.yaml
  ```

### Decrypting (gated reveal)
- Decryption in DDUI is **explicitly gated**:
  - Server-side must allow it: `DDUI_ALLOW_SOPS_DECRYPT=true`
  - UI sends a confirmation header: `X-Confirm-Reveal: yes`
  - Backend calls: `sops -d <file>` and returns the plaintext (not persisted).
- If decryption is not allowed you’ll see `403 Forbidden: decrypt disabled on server`.
- If SOPS fails, the backend returns the combined stderr/stdout so you can see the exact `sops` error.

**Security notes**
- DDUI never stores plaintext on disk—decrypt results stream back to the client only on explicit user action.
- Consider running the backend on a host you already trust with decryption keys, and restrict who can log in.

---

## Using DDUI
1. **Log in (OIDC)**. You’ll be redirected to `/auth/login` if no session.
2. **Add hosts to inventory.** Currently hosts are stored in the DB; the API supports reload from a path if you want to seed via file:
   ```bash
   # POST /api/inventory/reload with an optional { "path": "/data/inventory.yaml" }
   curl -sS -X POST -H "Content-Type: application/json" \
     -d '{"path":"/data/inventory.yaml"}' \
     http://localhost:8080/api/inventory/reload
   ```
   `inventory.yaml` (example) (Ansible formatted inventory, yaml/ini supported)
   ```yaml
   all:
     hosts:
   # GPU Accelerated:
       anchorage:
         ansible_host: 10.30.1.122
       leaf-cutter:
         ansible_host: 10.13.37.141
   ```
3. **Click Sync** on the Hosts page (or “Scan” per host). This will:
   - Scan IaC (`/data/docker-compose/...`), persist stacks/services/files.
   - Scan runtime per host (containers, images, ports, health).
4. **Drill into a host** to see:
   - Stacks merged from runtime and IaC.
   - For each row: name, state, image (runtime → desired), created, IP, ports (one per line), owner.
   - Per-host search box (filters rows).
5. **Metrics**: Hosts, Stacks, Containers, Drift, Errors aggregate across filtered hosts.
6. **SOPS**: encrypted `.env` files are detected (marked). Use the gated reveal if enabled.

---

## IaC layout details
- DDUI walks `<root>/<dirname>/<scope>/<stack>` (defaults `/data/docker-compose/*/*`).
- It records:
  - compose file (if present),
  - env files (SOPS detection via markers / file suffixes),
  - scripts `pre.sh`, `deploy.sh`, `post.sh`,
  - parsed services (image, labels, ports, volumes, env keys).
- **Scopes**
  - If `<scope>` equals a known host, it’s a host scope.
  - Otherwise it’s a group scope (applies to any host in that group).
- **Drift**
  - Different image than desired, a missing desired container/service, or IaC with no runtime ⇒ **drift**.

---

## Environment Variables

### Auth / OIDC

| Variable                                | Default                 | Description                                                                                 |
| --------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------- |
| `DDUI_BIND`                             | `0.0.0.0:8080` (example)| Server bind address                                                                         |
| `DDUI_COOKIE_DOMAIN`                    | empty                   | e.g. `.example.com`                                                                         |
| `DDUI_COOKIE_SECURE`                    | inferred                | `true/false` (if unset, inferred from redirect URL scheme)                                  |
| `DDUI_UI_DIR`                           | `/home/ddui/ui/dist`    | Where built SPA is served from                                                              |
| `UI_ORIGIN`                             | empty                   | Additional allowed CORS origin for the dev UI (`http://localhost:5173` is allowed by default) |
| `OIDC_ISSUER_URL`                       | —                       | Provider discovery URL (`…/.well-known/openid-configuration`)                               |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | —                       | OAuth client (secret supports `@/path` indirection)                                         |
| `OIDC_CLIENT_ID_FILE` / `OIDC_CLIENT_SECRET_FILE` | —                       | Same function as above but passed in as a file for docker secrets funtionality.   |
| `OIDC_REDIRECT_URL`                     | —                       | e.g. `http://localhost:8080/auth/callback`                                                  |
| `OIDC_SCOPES`                           | `openid email profile`  | Space-separated scopes                                                                      |
| `OIDC_ALLOWED_EMAIL_DOMAIN`             | empty                   | Restrict logins to a domain                                                                 |

### Database (Postgresql)

| Variable                    | Default | Description                                                                       |
| --------------------------- | ------- | --------------------------------------------------------------------------------- |
| `DDUI_DB_DSN`               | —       | Full connection string, e.g. `postgres://user:pass@host:5432/db?sslmode=disable`. |
| `DDUI_DB_HOST`              | —       | Hostname/IP of the database (used when DSN is not set).                           |
| `DDUI_DB_PORT`              | —       | Database port, e.g. `5432`.                                                       |
| `DDUI_DB_NAME`              | —       | Database name.                                                                    |
| `DDUI_DB_USER`              | —       | Database user.                                                                    |
| `DDUI_DB_PASS`              | —       | Database password (prefer `DDUI_DB_PASS_FILE` for secrets).                       |
| `DDUI_DB_PASS_FILE`         | —       | Read password from file (Docker secrets compatible).                              |
| `DDUI_DB_SSLMODE`           | —       | Postgres `sslmode` (`disable`, `require`, `verify-ca`, `verify-full`).            |
| `DDUI_DB_MAX_CONNS`         | —       | Max open connections in the pool (integer).                                       |
| `DDUI_DB_MIN_CONNS`         | —       | Minimum/idle pool size (integer).                                                 |
| `DDUI_DB_CONN_MAX_LIFETIME` | —       | Max lifetime per connection (duration, e.g. `30m`).                               |
| `DDUI_DB_CONN_MAX_IDLE`     | —       | Max idle time per connection (duration, e.g. `5m`).                               |
| `DDUI_DB_HEALTH_PERIOD`     | —       | Interval between DB health checks (duration, e.g. `10s`).                         |
| `DDUI_DB_CONNECT_TIMEOUT`   | —       | Dial/connect timeout (duration, e.g. `5s`).                                       |
| `DDUI_DB_PING_TIMEOUT`      | —       | Timeout for readiness/`PING` checks (duration, e.g. `2s`).                        |
| `DDUI_DB_MIGRATE`           | —       | `true/false` — run schema migrations on startup.                                  |

### Encryption & SOPS

| Variable                                | Default                 | Description                                                                                 |
| --------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------- |
| `DDUI_ALLOW_SOPS_DECRYPT`               | unset                   | Enable gated decrypt API (`true/1/yes/on`), requires `X-Confirm-Reveal: yes` header         |
| `SOPS_AGE_KEY_FILE` / `SOPS_AGE_KEY`    | unset                   | AGE private key (file path or raw), enables server-side **decrypt**                         |
| `SOPS_AGE_RECIPIENTS`                   | unset                   | Space-separated AGE recipients, enables **encrypt** even without `.sops.yaml`               |
| `DDUI_SESSION_SECRET`                   | —                       | Session/cookie HMAC secret. Generate via `DDUI_SESSION_SECRET="$(openssl rand -hex 64)"`    |
| `DDUI_SESSION_SECRET_FILE`              | —                       | Same function as above but passed in as a file for docker secrets funtionality.             |

### SSH Config

| Variable              | Default | Description                                                                   |
| --------------------- | ------- | ----------------------------------------------------------------------------- |
| `SSH_USER`            | —       | Remote username.                                                              |
| `SSH_PORT`            | —       | SSH port (e.g. `22`).                                                         |
| `SSH_KEY`             | —       | Inline private key (OpenSSH/PEM). Preserve newlines; prefer file for secrets. |
| `SSH_KEY_FILE`        | —       | Read private key from file (Docker secrets compatible).                       |
| `SSH_USE_SUDO`        | —       | `true/false` — run remote commands via `sudo`.                                |
| `SSH_STRICT_HOST_KEY` | —       | `true/false` — verify host key (disable to skip checks; not recommended).     |


### Auto DevOps

| Variable                                | Default                 | Description                                                                                 |
| --------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------- |
| `DDUI_SCAN_*`                           | see compose example     | Scan mode (local), root path, intervals, concurrency, host timeouts                         |

### Scanning Docker

| Variable                                | Default                 | Description                                                                                 |
| --------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------- |
| `DDUI_SCAN_*`                           | see compose example     | Scan mode (local), root path, intervals, concurrency, host timeouts                         |

### Scanning IaC

| Variable                                | Default                 | Description                                                                                 |
| --------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------- |
| `DDUI_IAC_ROOT`                         | `/data`                 | IaC repository root                                                                         |
| `DDUI_IAC_DIRNAME`                      | `docker-compose`        | Folder inside root DDUI scans                                                               |

---

## Contributing
- File issues with steps, logs, and versions.
- Small, focused PRs are best (typos, error handling, UI polish).
- Sample IaC directories welcome!
- Security-related PRs and hardening suggestions are especially appreciated (SOPS/AGE, cookie settings, RBAC, etc.).

---

## Support / Sponsorship
If you’d like to help keep the project moving:

[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/T6T41IT163)

---

## License (Open Core for non‑commercial use)
DDUI is offered under an **open-core, non‑commercial** model:

- For **home, personal, student, hobbyist, research, and other non‑commercial uses**, DDUI is free to use with **all features enabled**.
- For **commercial use** (including use inside a business, paid consulting, hosted/SaaS for customers, or any revenue‑generating context), please obtain a **commercial license** from the maintainer.

The project adopts the **Prosperity Public License 3.0.0 (Noncommercial)** as the baseline, plus DDUI‑specific **Additional Terms** to clarify that all features remain available to non‑commercial users. See `LICENSE.md` for details and contact information.

> _This section is a human‑readable summary and not a substitute for the license. Nothing here grants rights by itself._

---

## Disclaimer
The Software provided hereunder (“Software”) is licensed “as‑is,” without warranties of any kind—express, implied, or telepathically transmitted. The Softwarer (yes, that’s totally a word now) makes no promises about functionality, performance, compatibility, security, or availability—and absolutely no warranty of any sort. The developer shall not be held responsible, even if the software is clearly the reason your dog decided to orchestrate its own sidecar, your mom scored five tickets to Hawaii but you missed out because you were knee‑deep in a `docker compose` rabbit hole, or your stack drifted so hard it achieved sentience and renamed itself.

If using this orchestration UI leads you down a rabbit hole of obsessive network optimizations, breaks your fragile grasp of version pinning, or causes an uprising among your offline‑first containers—sorry, still not liable. Also not liable if your repo syncs so fast it rips a hole in the space‑time continuum **or** if your `.env` files multiply like Tribbles. The developer likewise claims no credit for anything that actually goes right either. Any positive experiences are owed entirely to the unstoppable force that is the Open Source community.

It’s never been a better time to be a PC user—or a homelabber. Just don’t blame me when YAML inevitably eats your weekend.
