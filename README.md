# <img src="ui/public/DD-UI-Logo.png" alt="DD-UI (Designated Driver UI)" width="35" height="35" /> DD-UI (Designated Driver UI)
> Declarative, security-first Docker orchestration. DD-UI compares runtime state (containers on your hosts) to declared state (your IaC repo), shows drift, and puts encryption (SOPS/AGE) and DevOps ergonomics first.

## What is DD-UI?
- Designated Driver UI is a Docker Managment Engine that puts DevOps and Encryption first.
- DD-UI seeks to ease the adoption of Infrastructure as Code and make it less intimidating for users to encrypt their secrets and sensitive docker values.
  - DD-UI manages all configuration of hosts/groups, and docker "stacks" as Inventory as Control (IaC) files. They are standardized format and CI/CD compatible. In essense DD-UI is a Docker focused CI/CD pipeline with a UI.
    - The state of your deployments is decoupled from the application and can be manipulated in the editor of your choice. DD-UI will redeploy containers when IaC files change.
  - DD-UI also allows you to decrypt/encrypt any IaC related files, you can deploy containers from encrypted docker-compose.yaml if you want.
    - This is good for those who like to stream while working on their servers or want to upload their compose and env to a repo as by default they are shown censored and they can be uploaded encrypted and dd-ui can actually deploy them if they are ever cloned and placed in its watch folder.
      - GitRepository syncing is currently unstable but will be fixed soon, but will allow backup/restore of stack configuration with ease.
- DD-UI seeks to bring the rewards of the DevOps mindset to those who may not have afforded them otherwise.
- DD-UI implements much of the features of other Docker GUIs and includes some industry tools like xterm 🔥 and monaco (editor used in vscode 🎉) to ensure a rich experience for the user.
- DD-UI is free forever, for non-commercial and home use. You can inquire for a commercial license. If you find us interesting feel free to give us a pull @ prplanit/dd-ui on the Docker Hub.

#### Thank you for your support!

<img src="example/screenshots/DD-UI-Decrypted-Deployed.png" width="400" /><img src="example/screenshots/DD-UI-Host-Stack_Detail-Collapsed.png" width="400" />
<img src="example/screenshots/DD-UI-Host-Stack_Detail-Expanded.png" width="400" /><img src="example/screenshots/DD-UI-Host-Stacks.png" width="400" />
<img src="example/screenshots/DD-UI-Hosts.png" width="400" /><img src="example/screenshots/DD-UI-Images.png" width="400" />
<img src="example/screenshots/DD-UI-Logs.png" width="400" /><img src="example/screenshots/DD-UI-Networks.png" width="400" />
<img src="example/screenshots/DD-UI-Stack-Compose-Editor.png" width="400" /><img src="example/screenshots/DD-UI-Stack-Encrypted.png" width="400" />
<img src="example/screenshots/DD-UI-Stats.png" width="400" /><img src="example/screenshots/DD-UI-Terminal.png" width="400" />
<img src="example/screenshots/DD-UI-Volumes.png" width="400" /><img src="example/screenshots/DD-UI-Cleanup.png" width="400" />
<img src="example/screenshots/DD-UI-Logging.png" width="400" />
---

## Status

### Nearing MVP / Pre-release
> DD-UI is 85-95% functional. Core functionality is present, there are some advanced features that are partially implemented. Known issues are listed below. CHEERS!

#### Project scope & cadence
- This project is built and maintained by **one person**. A large portion of the current codebase landed in **~1 week** of focused work.
- Development will continue and is driven by the maintainer’s available time.
- DD-UI is **opinionated**—it reflects how I run Docker in my homelab (declarative IaC, secrets via SOPS/AGE, minimal ceremony). If that resonates, you’ll likely feel right at home.
- I am working on two other projects currently, the most important one being one I am being paid to work on. And the other one is a *suprise* for some of you who endorsed my other projects. This will be running home prod for me so it does not make it any less of a priority, but now I have achieved functionality preview I am feeling I might slow the base and trickle minor bugfixes daily. It's honestly not that much left to do and I wanna make sure I am not neglecting the project I am getting paid for.

---

## What DD-UI does today
- Docker Management: Start/Stop/Pause/Resume/Kill containers.
- View live logs of any container (Dedicated Logging View, with advanced filters).
- Initiate a terminal session in a container. Uses xterm for a really rich experience in the shell.
- Edit docker compose, .env, and scripts. Application implements monaco editor (editor used in vscode) for a no compromise experience compared to other Docker management tools.
- **Inventory**: list hosts.
- **Stacks/Containers**: See all of your running docker containers in one view of all your combined systems.
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
- Docker Cleanup Page: Do the equivalent of a docker prune or clear your build cache from the comfort of the UI.

### Planned / Known Issues

- Bug when a file is open outside DD-UI it can create an empty temp file next to the file after saving.
- Maybe an enhanced approach for caching tags of orphaned / stranded images, the current approach for some images that are built at runtime it can be weird seeing it as just ?? in the menu. I want visibility for that.
- Groups and internal DD-UI variable are of the few things planned to test next. The GUI is ready, the inventory system can read and interpret all the files. I just need to validate that drift and prune is properly working and then its just putting this into home prod and seeing if it lets me down
- Perhaps a local admin user.
- A settings menu.
- A theme menu.
- More testing & bugfixes!
- Whatever idea I have that I suddenly think we can not live without!

Features are evolving; treat all APIs and UI as unstable for now.
Environment Variables are unlikely to change.

### Security

- We have done trivy and docker scout security tests and found no known vulnerabilities other than those which may be caused by our code. 
- I am not a CyberSecurity Admin more like a DevOps Engineer / SysAdmin cosplaying as a full-stack developer with the assistance of AI.
- We encourage anyone that is curious about our code and discovers any security issue to privately disclose that to us @ sofmeright@gmail.com or precisionplanit@gmail.com. We will do our best to resolve them as soon as possible.

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

---


**Point DD-UI at your IaC repo (local)**  
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
- DD-UI auto-detects if a scope matches a host in your inventory; otherwise it’s treated as a group.

Env (if you customize):
```bash
DD_UI_IAC_ROOT="/data"
DD_UI_IAC_DIRNAME="docker-compose"
# Gated decrypt is OFF by default; see SOPS section below to enable carefully.
# DD_UI_ALLOW_SOPS_DECRYPT=true
```

---

## Quick start (docker-compose)
This is a working docker-compose example.
Please edit the values to be specific to your deployment.
Don't forget to create the secret files and add the correct values.

```yaml
version: "3.8"
services:
  
  dd-ui-postgres:
    container_name: dd-ui-postgres
    image: postgres:16-alpine
    environment:
      - POSTGRES_DB=dd-ui
      - POSTGRES_USER=prplanit
      - POSTGRES_PASSWORD_FILE=/run/secrets/postgres_pass
    ports:
      - 5432:5432
    volumes:
      - /opt/docker/dd-ui/postgres:/var/lib/postgresql/data
    secrets:
      - postgres_pass
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $POSTGRES_USER -d $POSTGRES_DB"]
      interval: 5s
      timeout: 3s
      retries: 20
      
  dd-ui-app:
    container_name: dd-ui-app
    depends_on:
      dd-ui-postgres:
        condition: service_healthy
    image: prplanit/dd-ui:v0.4.7
    ports:
      - "3000:443"
    env_file: stack.env
    environment:
      # General Config
      #- DD_UI_BIND=0.0.0.0:443
      # - DD_UI_DEFAULT_OWNER= # (email)
      - DD_UI_INVENTORY_PATH=/data/inventory
      - DD_UI_LOCAL_HOST=anchorage
      - DD_UI_UI_ORIGIN=https://dd-ui.pcfae.com
      
      # Authentication / OIDC
      - DD_UI_COOKIE_SECURE=true
      - DD_UI_COOKIE_DOMAIN=dd-ui.pcfae.com
      - OIDC_CLIENT_ID_FILE=/run/secrets/oidc_client_id
      - OIDC_CLIENT_SECRET_FILE=/run/secrets/oidc_client_secret
      - OIDC_ISSUER_URL=https://sso.prplanit.com
      - OIDC_REDIRECT_URL=https://dd-ui.pcfae.com/auth/callback
      - OIDC_POST_LOGOUT_REDIRECT_URL=https://dd-ui.pcfae.com/login
      - OIDC_SCOPES=openid email profile
        # - OIDC_ALLOWED_EMAIL_DOMAIN # (optional; blocks others)
      
      # Database (Postgres) Configuration:
      - DD_UI_DB_HOST=dd-ui-postgres
      - DD_UI_DB_PORT=5432
      - DD_UI_DB_NAME=dd-ui
      - DD_UI_DB_USER=prplanit
      - DD_UI_DB_PASS_FILE=/run/secrets/postgres_pass
      - DD_UI_DB_SSLMODE=disable
      - DD_UI_DB_MIGRATE=true
        # or provide a single DSN:
        # - DD_UI_DB_DSN=postgres://dd-ui:...@db:5432/dd-ui?sslmode=disable

      # Docker Connection Config
      - DOCKER_CONNECTION_METHOD=local
      
      # Encryption / SOPS Config
      - DD_UI_ALLOW_SOPS_DECRYPT=true
      - SOPS_AGE_KEY_FILE=/run/secrets/sops_age_key
      - DD_UI_SESSION_SECRET_FILE=/run/secrets/session_secret
      
      # SSH Config
      - SSH_USER=kai           # or a limited user in docker group
      - SSH_PORT=22
      - SSH_KEY_FILE=/run/secrets/ssh_key
      - SSH_USE_SUDO=false      # true if your user needs sudo
      - SSH_STRICT_HOST_KEY=false
      
      # Auto DevOps Config
      - DD_UI_DEVOPS_APPLY=false
      
      # Scanning Config - Docker Host(s) States
      - DD_UI_SCAN_DOCKER_AUTO=true
      - DD_UI_SCAN_DOCKER_INTERVAL=1m
      - DD_UI_SCAN_DOCKER_HOST_TIMEOUT=45s
      - DD_UI_SCAN_DOCKER_CONCURRENCY=3
      - DD_UI_SCAN_DOCKER_ON_START=true
      - DD_UI_SCAN_DOCKER_DEBUG=true
      
      # Scannning Config - IAC
      - DD_UI_IAC_ROOT=/data
      - DD_UI_IAC_DIRNAME=docker-compose
      - DD_UI_SCAN_IAC_AUTO=true
      - DD_UI_SCAN_IAC_INTERVAL=90s

    secrets:
      - oidc_client_id
      - oidc_client_secret
      - postgres_pass
      - session_secret
      - sops_age_key
      - ssh_key
    volumes:
      - /opt/docker/dd-ui/data:/data
      - /var/run/docker.sock:/var/run/docker.sock

secrets:
  oidc_client_id:
    file: /opt/docker/dd-ui/secrets/oidc_client_id
  oidc_client_secret:
    file: /opt/docker/dd-ui/secrets/oidc_client_secret
  postgres_pass:
    file: /opt/docker/dd-ui/secrets/postgres_password
  session_secret:
    file: /opt/docker/dd-ui/secrets/session_secret
  sops_age_key:
    file: /opt/docker/dd-ui/secrets/sops_age_key
  ssh_key:
    file: /opt/docker/dd-ui/secrets/id_ed25519   # your private key
```

### `.env` file
```.env
POSTGRES_USER=prplanit
POSTGRES_DB=dd-ui
SOPS_AGE_RECIPIENTS=<placeyourkeyhere>
```

### `Nginx` Example:
```
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
  listen 80;
  listen [::]:80;

  server_name dd-ui.pcfae.com;
  return 301 https://$host$request_uri;
}

server {
  listen                    443 ssl http2;
  listen                    [::]:443 ssl http2;
  server_name               dd-ui.pcfae.com;
  # return 301                $scheme://dd-ui.pcfae.com$request_uri;

  access_log                /var/log/nginx/dd-ui.pcfae.com.access.log;
  error_log                 /var/log/nginx/dd-ui.pcfae.com.error.log;

  # TLS configuration
  # sudo openssl req -x509 -newkey rsa:4096 -keyout /etc/letsencrypt/live/172.122.122.104/privkey.pem -out /etc/letsencrypt/live/172.122.122.104/fullchain.pem -sha256 -days 3650 -nodes \
  # -subj "/C=XX/ST=Washington/L=Seattle/O=PrecisionPlanIT/OU=Internal/CN=cell-membrane"
  ssl_certificate           /etc/letsencrypt/live/pcfae.com/fullchain.pem;
  ssl_certificate_key       /etc/letsencrypt/live/pcfae.com/privkey.pem;
  ssl_protocols             TLSv1.2 TLSv1.3;

  ssl_ciphers 'ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:ECDHE-ECDSA-AES256-SHA384:ECDHE-RSA-AES256-SHA384';
  ssl_prefer_server_ciphers on;
  ssl_session_cache         shared:SSL:50m;
  ssl_session_timeout       1d;

  # OCSP Stapling ---
  # fetch OCSP records from URL in ssl_certificate and cache them
  ssl_stapling on;
  ssl_stapling_verify on;
  ssl_dhparam /etc/nginx/dhparam.pem;

  client_max_body_size 0;

  add_header 'Access-Control-Allow-Origin' 'https://apps.pcfae.com/';
  more_set_headers "Content-Security-Policy: form-action 'self' https://apps.pcfae.com/;";
  more_set_headers "Content-Security-Policy: frame-ancestors 'self' https://apps.pcfae.com/;";
  #add_header 'Content-Security-Policy' 'upgrade-insecure-requests';

  # WebSocket upgrade path (long-lived)
  location ^~ /api/ws/ {
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;

    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    proxy_read_timeout 3600s;

    # headroom for large Set-Cookie from upstream
    proxy_buffer_size   16k;
    proxy_buffers       8 32k;
    proxy_busy_buffers_size 64k;

    proxy_pass https://anchorage:3000;
  }

  location / {

    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;

    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    proxy_read_timeout 3600s;

    # headroom for large Set-Cookie from upstream
    proxy_buffer_size   16k;
    proxy_buffers       8 32k;
    proxy_busy_buffers_size 64k;

    # proxy_redirect off;
    proxy_pass https://anchorage:3000/;
  }
}
```

## Quick start (developer mode)
> Best for hacking on the UI/API locally.

## Requirements
- Docker reachable from the DD-UI backend to each host you list (TCP or local socket).
- PostgreSQL 14+
- Node 18+ (for dev UI), Go 1.21+ (backend)
- OIDC provider (tested with Zitadel) or run in “local only” with `/api/session` returning no user (login page will redirect).
- **SOPS 3.10+** available on the backend host/container (DD-UI calls `sops` by name).  
  The provided Docker image installs it to `/usr/local/bin/sops`.

1) **Postgres**
```bash
docker run -d --name dd-ui-pg -p 5432:5432 \
  -e POSTGRES_PASSWORD=devpass -e POSTGRES_USER=dd-ui -e POSTGRES_DB=dd-ui \
  postgres:15
```
Set `DATABASE_URL` for the backend:
```bash
export DATABASE_URL=postgres://dd-ui:devpass@localhost:5432/dd-ui?sslmode=disable
```

2) **OIDC (Zitadel example)**  
Create an OAuth 2.0 Web client:
- Redirect URL: `https://your-dd-ui.example.com/auth/callback` (or `http://localhost:8080/auth/callback` for dev)
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
# If unset, DD-UI infers COOKIE_SECURE from the redirect URL scheme
# export COOKIE_SECURE=true|false
```

3) **Run backend**
```bash
cd src/api
go run .
# or: go build -o dd-ui && ./dd-ui
```
The backend runs DB migrations automatically at startup (ensure `DATABASE_URL` is set).

4) **Run frontend**
```bash
cd ui
pnpm install
pnpm dev
```
Visit `http://localhost:5173` (or the port Vite prints).  
In production, the Go server serves the built UI; during dev it’s fine to run separately.

Build the UI once:
```bash
cd ui && pnpm install && pnpm build
```
Then hit `http://localhost:8080` (or `http://localhost:3000` if you used the mapping above).

---

## SOPS / AGE: keys, encrypt, decrypt

DD-UI integrates with **SOPS** to keep secrets encrypted at rest in your IaC repo. The backend calls the `sops` CLI; ensure it’s on `PATH` (our image installs it to `/usr/local/bin/sops`).

### Generate AGE key pair (server-side decrypt capability)
On a **secure workstation** or secrets box:
```bash
# Generate a private key; prints the public recipient on stderr
age-keygen -o /opt/docker/dd-ui/secrets/sops_age_key.txt
# Show (or copy) the public recipient for encryption (starts with "age1")
age-keygen -y /opt/docker/dd-ui/secrets/sops_age_key.txt
```
Wire it into Compose as a Docker secret (see `sops_age_key` in the compose file above), and point DD-UI at it:
```
SOPS_AGE_KEY_FILE=/run/secrets/sops_age_key
```
> **Never** commit the private key to Git. Treat `/opt/docker/dd-ui/secrets/sops_age_key.txt` like any other production secret.

### Choose encrypt recipients
To encrypt, SOPS needs one or more **AGE recipients** (public keys). You have two main options:

1. **Environment variable (no repo config required)**  
   Set `SOPS_AGE_RECIPIENTS` with one or more recipients (space-separated):
   ```
   SOPS_AGE_RECIPIENTS="age1teamUser1... age1teamUser2... age1ciKey..."
   ```
   DD-UI will pass each recipient to `sops` as `--age <recipient>` during encryption.

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
       # (DD-UI already hints this when encrypting *.env)
       # unencrypted_suffix: _unencrypted
   ```

> If you see `sops: encrypt failed: ... config file not found, or has no creation rules, and no keys provided ...`, it means neither `SOPS_AGE_RECIPIENTS` nor a `.sops.yaml` with matching creation rules were found. Provide recipients or add a config.

### Encrypting files
- **From the DD-UI UI**: creating/updating a file with the **SOPS** toggle ON (or naming it `*_private.env` / `*_secret.env`) will attempt to run:
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
- Decryption in DD-UI is **explicitly gated**:
  - Server-side must allow it: `DD_UI_ALLOW_SOPS_DECRYPT=true`
  - UI sends a confirmation header: `X-Confirm-Reveal: yes`
  - Backend calls: `sops -d <file>` and returns the plaintext (not persisted).
- If decryption is not allowed you’ll see `403 Forbidden: decrypt disabled on server`.
- If SOPS fails, the backend returns the combined stderr/stdout so you can see the exact `sops` error.

**Security notes**
- DD-UI never stores plaintext on disk—decrypt results stream back to the client only on explicit user action.
- Consider running the backend on a host you already trust with decryption keys, and restrict who can log in.

---

## Using DD-UI
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
- DD-UI walks `<root>/<dirname>/<scope>/<stack>` (defaults `/data/docker-compose/*/*`).
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

### General

| Variable              | Default | Description                                                                                      |
| --------------------- | ------- | ------------------------------------------------------------------------------------------------ |
| `DD_UI_DEFAULT_OWNER`  | —       | Default owner/team used when creating stacks or records (namespacing/attribution in the UI).     |
| `DD_UI_BUILDS_DIR`     | —       | Directory for build outputs and artifacts (e.g., generated bundles/manifests).                   |
| `DD_UI_INVENTORY_PATH` | —       | Path to the hosts inventory file (YAML/JSON) defining remote Docker targets.                     |
| `DD_UI_LOCAL_HOST`     | `""`    | Optional override for the local host name/label; leave empty to use the tool’s implicit/default. |
| `DD_UI_BIND`           | —       | Server bind address, e.g. `:8080` or `0.0.0.0:8080`.                                             |
| `DD_UI_UI_ORIGIN`                             | empty                   | Additional allowed CORS origin for the dev UI (`http://localhost:5173` is allowed by default) |
| `DD_UI_UI_DIR`                           | `/home/dd-ui/ui/dist`    | Where built SPA is served from                                                              |


### Auth / OIDC

| Variable                                | Default                 | Description                                                                                 |
| --------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------- |
| `DD_UI_COOKIE_DOMAIN`                    | empty                   | e.g. `.example.com`                                                                         |
| `DD_UI_COOKIE_SECURE`                    | inferred                | `true/false` (if unset, inferred from redirect URL scheme)                                  |
| `OIDC_ISSUER_URL`                       | —                       | Provider discovery URL (`…/.well-known/openid-configuration`)                               |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | —                       | OAuth client (secret supports `@/path` indirection)                                         |
| `OIDC_CLIENT_ID_FILE` / `OIDC_CLIENT_SECRET_FILE` | —                       | Same function as above but passed in as a file for docker secrets funtionality.   |
| `OIDC_REDIRECT_URL`                     | —                       | e.g. `http://localhost:8080/auth/callback`                                                  |
| `OIDC_SCOPES`                           | `openid email profile`  | Space-separated scopes                                                                      |
| `OIDC_ALLOWED_EMAIL_DOMAIN`             | empty                   | Restrict logins to a domain                                                                 |

### Database (Postgresql)

| Variable                    | Default | Description                                                                       |
| --------------------------- | ------- | --------------------------------------------------------------------------------- |
| `DD_UI_DB_DSN`               | —       | Full connection string, e.g. `postgres://user:pass@host:5432/db?sslmode=disable`. |
| `DD_UI_DB_HOST`              | —       | Hostname/IP of the database (used when DSN is not set).                           |
| `DD_UI_DB_PORT`              | —       | Database port, e.g. `5432`.                                                       |
| `DD_UI_DB_NAME`              | —       | Database name.                                                                    |
| `DD_UI_DB_USER`              | —       | Database user.                                                                    |
| `DD_UI_DB_PASS`              | —       | Database password (prefer `DD_UI_DB_PASS_FILE` for secrets).                       |
| `DD_UI_DB_PASS_FILE`         | —       | Read password from file (Docker secrets compatible).                              |
| `DD_UI_DB_SSLMODE`           | —       | Postgres `sslmode` (`disable`, `require`, `verify-ca`, `verify-full`).            |
| `DD_UI_DB_MAX_CONNS`         | —       | Max open connections in the pool (integer).                                       |
| `DD_UI_DB_MIN_CONNS`         | —       | Minimum/idle pool size (integer).                                                 |
| `DD_UI_DB_CONN_MAX_LIFETIME` | —       | Max lifetime per connection (duration, e.g. `30m`).                               |
| `DD_UI_DB_CONN_MAX_IDLE`     | —       | Max idle time per connection (duration, e.g. `5m`).                               |
| `DD_UI_DB_HEALTH_PERIOD`     | —       | Interval between DB health checks (duration, e.g. `10s`).                         |
| `DD_UI_DB_CONNECT_TIMEOUT`   | —       | Dial/connect timeout (duration, e.g. `5s`).                                       |
| `DD_UI_DB_PING_TIMEOUT`      | —       | Timeout for readiness/`PING` checks (duration, e.g. `2s`).                        |
| `DD_UI_DB_MIGRATE`           | —       | `true/false` — run schema migrations on startup.                                  |

### Docker Connection Config

| Variable                   | Default                | Description                                                                                                                                                     |
| -------------------------- | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DOCKER_CONNECTION_METHOD` | `ssh`                  | How to connect to Docker: `ssh`, `tcp`, or `local` (Unix socket).                                                                                               |
| `DOCKER_SOCK_PATH`         | `/var/run/docker.sock` | Path to local Docker socket (used when method=`local`).                                                                                                         |
| `DOCKER_TCP_PORT`          | `2375`                 | Docker TCP port (used when method=`tcp`).                                                                                                                       |
| `SSH_USER`                 | `root`                 | Remote user for SSH Docker connections (see **SSH (Remote)** for keys/port options).                                                                            |
| `DOCKER_SSH_CMD`           | —                      | Advanced override: full SSH command (binary + flags). If set, it supersedes `SSH_*` vars. E.g. `ssh -i /run/secrets/ssh_key -p 22 -o StrictHostKeyChecking=no`. |


### Encryption & SOPS

| Variable                                | Default                 | Description                                                                                 |
| --------------------------------------- | ----------------------- | ------------------------------------------------------------------------------------------- |
| `DD_UI_ALLOW_SOPS_DECRYPT`               | unset                   | Enable gated decrypt API (`true/1/yes/on`), requires `X-Confirm-Reveal: yes` header         |
| `SOPS_AGE_KEY_FILE` / `SOPS_AGE_KEY`    | unset                   | AGE private key (file path or raw), enables server-side **decrypt**                         |
| `SOPS_AGE_RECIPIENTS`                   | unset                   | Space-separated AGE recipients, enables **encrypt** even without `.sops.yaml`               |
| `DD_UI_SESSION_SECRET`                   | —                       | Session/cookie HMAC secret. Generate via `DD_UI_SESSION_SECRET="$(openssl rand -hex 64)"`    |
| `DD_UI_SESSION_SECRET_FILE`              | —                       | Same function as above but passed in as a file for docker secrets funtionality.             |

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

| Variable                                | Default                 | Description                                                            |
| --------------------------------------- | ----------------------- | ---------------------------------------------------------------------- |
| `DD_UI_DEVOPS_APPLY`                     | `true`                  | Enables Automated Deployments via IaC / DevOps                         |

### Scanning Docker

| Variable                        | Default | Description                                                   |
| ------------------------------- | ------- | ------------------------------------------------------------- |
| `DD_UI_SCAN_DOCKER_AUTO`         | `true`  | `true/false` — enable the periodic Docker scan scheduler.     |
| `DD_UI_SCAN_DOCKER_INTERVAL`     | `1m`    | How often to run scans (Go duration, e.g. `30s`, `5m`, `1h`). |
| `DD_UI_SCAN_DOCKER_HOST_TIMEOUT` | `45s`   | Per-host scan timeout (Go duration).                          |
| `DD_UI_SCAN_DOCKER_CONCURRENCY`  | `3`     | Max number of hosts scanned in parallel (integer).            |
| `DD_UI_SCAN_DOCKER_ON_START`     | `true`  | `true/false` — run an initial scan at startup.                |
| `DD_UI_SCAN_DOCKER_DEBUG`        | `false` | `true/false` — verbose logging for the Docker scanner.        |


### Scanning IaC

| Variable                 | Default | Description                                                                             |
| ------------------------ | ------- | --------------------------------------------------------------------------------------- |
| `DD_UI_SCAN_IAC_AUTO`     | `true`  | `true/false` — enable the periodic IaC (compose) scan scheduler.                        |
| `DD_UI_SCAN_IAC_INTERVAL` | `90s`   | How often to run IaC scans (Go duration, e.g. `30s`, `5m`, `1h`).                       |
| `DD_UI_IAC_ROOT`          | —       | Root path to scan for IaC (Docker Compose) files; recommended `/data`.   |
| `DD_UI_IAC_DIRNAME`       | `empty` | Optional subfolder under the root to scope scans; leave empty to use the root directly; recommended `docker-compose`. |

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
DD-UI is offered under an **open-core, non‑commercial** model:

- For **home, personal, student, hobbyist, research, and other non‑commercial uses**, DD-UI is free to use with **all features enabled**.
- For **commercial use** (including use inside a business, paid consulting, hosted/SaaS for customers, or any revenue‑generating context), please obtain a **commercial license** from the maintainer.

The project adopts the **Prosperity Public License 3.0.0 (Noncommercial)** as the baseline, plus DD-UI‑specific **Additional Terms** to clarify that all features remain available to non‑commercial users. See `LICENSE.md` for details and contact information.

> _This section is a human‑readable summary and not a substitute for the license. Nothing here grants rights by itself._

---

## Disclaimer
The Software provided hereunder (“Software”) is licensed “as‑is,” without warranties of any kind—express, implied, or telepathically transmitted. The Softwarer (yes, that’s totally a word now) makes no promises about functionality, performance, compatibility, security, or availability—and absolutely no warranty of any sort. The developer shall not be held responsible, even if the software is clearly the reason your dog decided to orchestrate its own sidecar, your mom scored five tickets to Hawaii but you missed out because you were knee‑deep in a `docker compose` rabbit hole, or your stack drifted so hard it achieved sentience and renamed itself.

If using this orchestration UI leads you down a rabbit hole of obsessive network optimizations, breaks your fragile grasp of version pinning, or causes an uprising among your offline‑first containers—sorry, still not liable. Also not liable if your repo syncs so fast it rips a hole in the space‑time continuum **or** if your `.env` files multiply like Tribbles. The developer likewise claims no credit for anything that actually goes right either. Any positive experiences are owed entirely to the unstoppable force that is the Open Source community.

It’s never been a better time to be a PC user—or a homelabber. Just don’t blame me when YAML inevitably eats your weekend.
