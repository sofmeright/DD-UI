# DDUI Deployment Notes

## Docker Compose Deployment

When running the DDUI application, use the proper environment file and test configuration:

```bash
# Correct command with environment file and test compose file
docker compose --env-file stack.env -f docker-compose.test.yml up -d

# Alternative if using default docker-compose.yml, ensure stack.env is sourced
source stack.env && docker compose up -d
```

## Key Files
- `stack.env` - Contains environment variables needed for proper configuration
- `docker-compose.test.yml` - Test compose file (if using test setup)
- `docker-compose.yml` - Default compose file

## Recent Changes Made
- ✅ Fixed endless container inspection API spam during deployments
- ✅ Optimized polling behavior (30s monitoring, every 5s, with reduced inspection frequency)  
- ✅ Added proper deployment feedback:
  - "⏳ Deployment initiated..." when starting
  - "🎉 Deployment completed successfully!" when all containers running
  - "❌ Deployment failed..." when containers fail
  - Manual dismiss button (×) on all deployment messages
- ✅ Consolidated database migrations from 15+ files to 7 logical migrations
- ✅ Fixed database schema to match current Go implementation exactly
- ✅ Fixed migration dependency order (deployment_stamps now created after iac_stacks)

## Database Migration Policy
**PERFECT INITIAL DATABASE DESIGN RULE**: 
- Keep migration count minimal (currently 7 files)
- Systematically wire database & migrations to match current working Go implementation
- Fix schema mismatches by editing existing migration files, not creating new ones
- This rule stays active until explicitly disabled by user
- All changes must ensure fresh database deployments work correctly

## Database Schema Reference
**Complete table structure as of current implementation:**

### Core Infrastructure (001_core_schema.sql)
- `hosts` - Physical/virtual hosts with metadata (id, name, addr, vars, groups, labels, owner)
- `stacks` - Runtime stack inventory per host (id, host_id, project, source, owner, auto_apply_override, iac_enabled)
- `containers` - Container runtime state (id, host_id, stack_id, container_id, name, image, state, status, ports, labels, deployment_stamp_id, deployment_hash)
- `set_updated_at()` function - Trigger function for timestamp updates

### Runtime Inventory (002_runtime_inventory.sql)  
- `image_tags` - Docker image registry tracking (host_name, image_id, repo, tag, first_seen, last_seen)

### Infrastructure as Code System (003_iac_system.sql)
- `iac_repos` - Repository definitions (id, kind, root_path, url, branch, enabled)  
- `iac_stacks` - Stack definitions from IaC scanning (id, repo_id, scope_kind, scope_name, stack_name, compose_file, deploy_kind, sops_status, auto_apply_override, **auto_devops_override**)
- `iac_services` - Services within stacks (id, stack_id, service_name, container_name, image, labels, env_keys, ports, volumes)
- `iac_stack_files` - File tracking for stacks (id, stack_id, role, rel_path, sops, sha256_hex, size_bytes)
- `iac_deployments` - Deployment execution records (id, stack_id, hosts, output, success, started_at, completed_at)
- `deployment_stamps` - Deployment tracking with status (id, host_id, stack_id, deployment_hash, deployment_method, deployment_user, deployment_status)
- `iac_overrides` - Override system for auto-devops policy (id, level, scope_name, stack_name, key, value, auto_devops_override) 
- `settings` - Global key-value settings (id, key, value)

### Monitoring & Logs (004_monitoring_logs.sql)
- `scan_logs` - System scanning and operation logs (id, host_id, at, level, message, data)

### Application Settings (005_app_settings.sql)
- `app_settings` - Application configuration key-value store (key, value)
- `host_settings` - Per-host override settings (host_name, auto_apply_override)
- `group_settings` - Per-group override settings (group_name, auto_apply_override)

### Deployment Tracking (006_deployment_tracking.sql)  
- Empty migration file - deployment tracking consolidated into 003_iac_system.sql

### Stack Drift Detection (007_stack_drift_cache.sql)
- `stack_drift_cache` - Hash-based drift detection cache (stack_id, bundle_hash, docker_config_cache, last_updated)

**Key Relationships:**
- hosts(id) ← containers(host_id), deployment_stamps(host_id), scan_logs(host_id)  
- iac_stacks(id) ← deployment_stamps(stack_id), iac_services(stack_id), stack_drift_cache(stack_id)
- deployment_stamps(id) ← containers(deployment_stamp_id)
- iac_repos(id) ← iac_stacks(repo_id)

## Database Migrations
Migration files are now consolidated and properly match the current Go code:
- 001_core_schema.sql - Hosts, stacks, containers, common functions
- 002_runtime_inventory.sql - Deployment stamps, image tracking
- 003_iac_system.sql - Infrastructure as Code tables  
- 004_monitoring_logs.sql - Scan logs (with proper host_id reference)
- 005_app_settings.sql - Key-value settings tables
- 006_deployment_tracking.sql - (consolidated into 002)
- 007_stack_drift_cache.sql - Hash-based drift detection