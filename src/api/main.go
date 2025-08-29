package main

import (
    "log"
    "net/http"
    "time"
)

var startedAt = time.Now()

func main() {
    if err := InitAuthFromEnv(); err != nil {
        log.Fatalf("OIDC setup failed: %v", err)
    }
    addr := env("DDUI_BIND", ":8080")
    uiDir := env("DDUI_UI_DIR", "/home/ddui/ui/dist")
    log.Printf("DDUI API on %s (ui=%s)", addr, uiDir)

    log.Fatal(http.ListenAndServe(addr, makeRouter()))
}