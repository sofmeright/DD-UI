package main

import (
	"log"
	"net/http"
	"time"
)

var startedAt time.Time

func main() {
	startedAt = time.Now()

	// OIDC / session store
	if err := InitAuthFromEnv(); err != nil {
		log.Fatalf("OIDC setup failed: %v", err)
	}

	addr := env("DDUI_BIND", ":8080")
	uiDir := env("DDUI_UI_DIR", "/home/ddui/ui/dist")
	log.Printf("DDUI API on %s (ui=%s)", addr, uiDir)

	// makeRouter lives in web.go
	if err := http.ListenAndServe(addr, makeRouter()); err != nil {
		log.Fatal(err)
	}
}
