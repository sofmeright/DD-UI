package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connection pool used across handlers
var db *pgxpool.Pool

// --- ENV helpers ---

func dsnFromEnv() string {
	if s := env("DDUI_DB_DSN", ""); s != "" {
		return s
	}
	host := env("DDUI_DB_HOST", "postgres")
	port := env("DDUI_DB_PORT", "5432")
	user := env("DDUI_DB_USER", "ddui")
	pass := env("DDUI_DB_PASS", "ddui")
	name := env("DDUI_DB_NAME", "ddui")
	ssl := env("DDUI_DB_SSLMODE", "disable") // "require" if you run TLS
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
}

func InitDBFromEnv(ctx context.Context) error {
	cfg, err := pgxpool.ParseConfig(dsnFromEnv())
	if err != nil {
		return err
	}
	if n, err := strconv.Atoi(env("DDUI_DB_MAX_CONNS", "12")); err == nil {
		cfg.MaxConns = int32(n)
	}
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return err
	}
	db = pool
	log.Printf("db: connected to Postgres (max_conns=%d)", cfg.MaxConns)

	if strings.ToLower(env("DDUI_DB_MIGRATE", "true")) == "true" {
		if err := runMigrations(ctx, db); err != nil {
			return fmt.Errorf("migrations: %w", err)
		}
	}
	return nil
}

// --- Migrations ---

//go:embed migrations/*.sql
var migrationsFS embed.FS

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version int PRIMARY KEY)`); err != nil {
		return err
	}
	var current int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	type mig struct {
		v    int
		name string
	}
	var list []mig
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".sql") {
			continue
		}
		// files like 001_init.sql, 002_more.sql
		base := strings.SplitN(n, "_", 2)[0]
		v, _ := strconv.Atoi(strings.TrimLeft(base, "0"))
		if v == 0 && base != "0" {
			continue
		}
		if v > current {
			list = append(list, mig{v: v, name: n})
		}
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
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, m.v); err != nil {
			return err
		}
		log.Printf("db: applied migration %s", m.name)
	}

	return tx.Commit(ctx)
}
