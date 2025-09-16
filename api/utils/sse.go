// utils/sse.go
package utils

import (
	"net/http"
	"strings"
	"sync"

	"dd-ui/common"
	"github.com/gorilla/websocket"
)

// SSELineWriter wraps an http.ResponseWriter to send streaming lines as SSE events
type SSELineWriter struct {
	mu     sync.Mutex
	w      http.ResponseWriter
	fl     http.Flusher
	stream string // "stdout" | "stderr"
	buf    []byte
}

// NewSSELineWriter creates a new SSE line writer for the given stream type
func NewSSELineWriter(w http.ResponseWriter, fl http.Flusher, stream string) *SSELineWriter {
	return &SSELineWriter{
		w:      w,
		fl:     fl,
		stream: stream,
	}
}

// Write implements io.Writer, buffering input and sending complete lines as SSE events
func (s *SSELineWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	for {
		i := -1
		for j, b := range s.buf {
			if b == '\n' {
				i = j
				break
			}
		}
		if i == -1 {
			break
		}
		line := string(s.buf[:i])
		s.buf = s.buf[i+1:]
		_, _ = s.w.Write([]byte("event: " + s.stream + "\n"))
		_, _ = s.w.Write([]byte("data: " + line + "\n\n"))
		if s.fl != nil {
			s.fl.Flush()
		}
	}
	return len(p), nil
}

// WriteSSEHeader sets the necessary headers for Server-Sent Events and returns a flusher
func WriteSSEHeader(w http.ResponseWriter) (http.Flusher, bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Disable proxy buffering (nginx)
	w.Header().Set("X-Accel-Buffering", "no")
	fl, ok := w.(http.Flusher)
	return fl, ok
}

// WSUpgrader provides a configured websocket upgrader
var WSUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		// allow same-origin and configured UI origin
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		ui := strings.TrimSpace(common.Env("DD_UI_UI_ORIGIN", ""))
		if origin == "" || origin == ui {
			return true
		}
		// dev helpers
		if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
			return true
		}
		return false
	},
}