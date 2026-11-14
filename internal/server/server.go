package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// New returns the application HTTP handler.
func New() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "goexpert-rate-limiter",
			"status":  "ok",
		})
	})

	return http.TimeoutHandler(mux, 10*time.Second, `{"error":"request timeout"}`)
}
