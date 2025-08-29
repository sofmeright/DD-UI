package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Exported pool for handlers.
var db *pgxpool.Pool

// ---- ENV helpers ----

func dsnFromEnv() string {
	if s := env("DDUI_DB_DSN", ""); s != "" {
		return s
	}
	// Convenient defaults for dev; override with DDUI_DB_* or DDUI_DB_DSN.
	host := env("DDUI_DB_HOST", "postgres")
	port := env("DDUI_DB_PORT", "5432")
	user := env("DDUI_DB_USER", "ddui")
	pass := env("DDUI_DB_PASS", "ddui")
	name := env("DDUI_DB_NAME", "ddui")
	ssl := env("DDUI_DB_SSLMODE", "disable") // set "require" if TLS
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
}

func atoiEnv(key, def string) int {
	if v := env(key, def); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	n, _ := strconv.Atoi(def)
	return n
}

func durEnv(key string, def time.Duration) time.Duration {
	if v := env(key, ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// ---- Init / Close ----

func InitDBFromEnv(ctx context.Context) error {
	dsn := dsnFromEnv()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	// Pool tuning (override via env)
	cfg.MaxConns = int32(atoiEnv("DDUI_DB_MAX_CONNS", "20"))
	minConns := atoiEnv("DDUI_DB_MIN_CONNS", "2")
	cfg.MinConns = int32(minConns)
	cfg.MaxConnLifetime = durEnv("DDUI_DB_CONN_MAX_LIFETIME", time.Hour)   // recycle long-lived conns
	cfg.MaxConnIdleTime = durEnv("DDUI_DB_CONN_MAX_IDLE", 30*time.Minute) // close idles
	cfg.HealthCheckPeriod = durEnv("DDUI_DB_HEALTH_PERIOD", 30*time.Second)

	// Dial/connect timeout for new physical conns.
	// (pgxpool.Config embeds a ConnConfig; ConnectTimeout is honored during new connections)
	cfg.ConnConfig.ConnectTimeout = durEnv("DDUI_DB_CONNECT_TIMEOUT", 5*time.Second)

	// Create pool
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("new pool: %w", err)
	}

	// Quick ping with timeout
	pingCtx, cancel := context.WithTimeout(ctx, durEnv("DDUI_DB_PING_TIMEOUT", 5*time.Second))
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return fmt.Errorf("ping: %w", err)
	}

	DB = pool
	log.Printf("db: connected (max=%d min=%d idle=%s lifetime=%s)",
		cfg.MaxConns, cfg.MinConns, cfg.MaxConnIdleTime, cfg.MaxConnLifetime)

	// Migrate (opt-out with DDUI_DB_MIGRATE=false)
	if strings.ToLower(env("DDUI_DB_MIGRATE", "true")) == "true" {
		if err := runMigrations(ctx, DB); err != nil {
			return fmt.Errorf("migrations: %w", err)
		}
	}

	return nil
}

func CloseDB() {
	if DB != nil {
		DB.Close()
	}
}

func DBReady() bool { return DB != nil }

// ---- Migrations ----

//go:embed migrations/*.sql
var migrationsFS embed.FS

// runMigrations applies any *.sql in the embedded migrations/ folder that have a
// version prefix (e.g. 001_init.sql). It uses a DB-wide advisory lock so multiple
// DDUI instances won't race on startup.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Take a global advisory lock (arbitrary constant "DDUI")
	const migLock int64 = 727171182
	if _, err := pool.Exec(ctx, `select pg_advisory_lock($1)`, migLock); err != nil {
		return fmt.Errorf("advisory_lock: %w", err)
	}
	defer pool.Exec(context.Background(), `select pg_advisory_unlock($1)`, migLock)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		create table if not exists schema_migrations(
			version int primary key,
			applied_at timestamptz not null default now()
		)`); err != nil {
		return err
	}

	var current int
	if err := tx.QueryRow(ctx, `select coalesce(max(version), 0) from schema_migrations`).Scan(&current); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		// If no embedded dir exists, just skip (useful in early dev)
		if errorsIs(err, fs.ErrNotExist) {
			log.Printf("db: no embedded migrations found (skipping)")
			return tx.Commit(ctx)
		}
		return err
	}

	type mig struct {
		v    int
		name string
	}
	var list []mig
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".sql") {
			continue
		}
		base := strings.SplitN(n, "_", 2)[0]
		v, _ := strconv.Atoi(strings.TrimLeft(base, "0"))
		// Accept "000.sql" or "0_*.sql" as version 0 if desired
		if v == 0 && base != "0" && base != "000" {
			continue
		}
		if v > current {
			list = append(list, mig{v: v, name: n})
		}
	}
	if len(list) == 0 {
		log.Printf("db: migrations up-to-date (version=%d)", current)
		return tx.Commit(ctx)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v < list[j].v })

	for _, m := range list {
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("%s: %w", m.name, err)
		}
		if _, err := tx.Exec(ctx, `insert into schema_migrations(version) values($1)`, m.v); err != nil {
			return err
		}
		log.Printf("db: applied migration %s", m.name)
	}

	return tx.Commit(ctx)
}

// tiny helper because we don't want to import "errors" all over if already present elsewhere
func errorsIs(err error, target error) bool {
	type causer interface{ Is(error) bool }
	if err == nil {
		return target == nil
	}
	if ce, ok := err.(causer); ok && ce.Is(target) {
		return true
	}
	return err == target
}
