package middleware

import (
	"io"
	"log"
	"net"
	"net/http"

	"github.com/natanzaum/goexpert-rate-limiter/internal/domain"
)

// DefaultBlockedMessage is returned verbatim when a request is blocked.
const DefaultBlockedMessage = "you have reached the maximum number of requests or actions allowed within a certain time frame"

// Options configures the middleware.
type Options struct {
	// TokenHeader is the header used to read the API key. Defaults to API_KEY.
	TokenHeader string
}

// RateLimit builds the HTTP middleware. It only extracts the request identity
// and translates the business decision into an HTTP response; all rate limiting
// rules live in domain.Service.
func RateLimit(service *domain.Service, opts Options) func(http.Handler) http.Handler {
	header := opts.TokenHeader
	if header == "" {
		header = "API_KEY"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			token := r.Header.Get(header)

			result, err := service.Check(r.Context(), ip, token)
			if err != nil {
				log.Printf("rate limiter: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

			decision := result.Decision

			if !decision.Allowed {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = io.WriteString(w, DefaultBlockedMessage)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
