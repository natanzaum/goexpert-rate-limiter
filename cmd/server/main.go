package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/natanzaum/goexpert-rate-limiter/internal/config"
	"github.com/natanzaum/goexpert-rate-limiter/internal/domain"
	"github.com/natanzaum/goexpert-rate-limiter/internal/middleware"
	"github.com/natanzaum/goexpert-rate-limiter/internal/server"
	redisstore "github.com/natanzaum/goexpert-rate-limiter/internal/strategy/redis"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[rate-limiter] ")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	redisStore, err := redisstore.New(cfg.Redis)
	if err != nil {
		log.Fatalf("could not initialize rate limiter store: %v", err)
	}
	defer func() {
		if err := redisStore.Close(); err != nil {
			log.Printf("could not close redis connection: %v", err)
		}
	}()
	log.Printf("connected to redis at %s (db %d)", cfg.Redis.Addr, cfg.Redis.DB)

	limiter := domain.NewService(redisStore, cfg.Limiter)

	handler := middleware.RateLimit(limiter, middleware.Options{
		TokenHeader: cfg.TokenHeader,
	})(server.New())

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("listening on %s (ip limit: %d req/s, block: %s, token header: %s)",
			httpServer.Addr, cfg.Limiter.IPLimit, cfg.Limiter.BlockTime, cfg.TokenHeader)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Print("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}
