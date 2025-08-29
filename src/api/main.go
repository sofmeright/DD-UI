package main

import (
    "log"
    "net/http"
    "time"
)

var startedAt = time.Now()

func main() {
	addr := env("DDUI_BIND", ":8080")

	if err := InitAuthFromEnv(); err != nil {
		log.Fatalf("OIDC setup failed: %v", err)
	}
    ctx := context.Background()
	if err := InitDBFromEnv(ctx); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	if err := InitInventory(); err != nil {
		log.Fatalf("inventory init failed: %v", err)
	}
    log.Printf("DDUI API on %s (ui=/home/ddui/ui/dist)", addr)
    if err := http.ListenAndServe(addr, makeRouter()); err != nil {
        log.Fatal(err)
    }
}