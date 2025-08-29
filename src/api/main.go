package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

var startedAt = time.Now()

func main() {
	addr := getEnv("DDUI_ADDR", ":8080")
	r := makeRouter()
	log.Printf("DDUI listening on %s\n", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
