package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lixiansheng/fileflow/internal/auth"
	"github.com/lixiansheng/fileflow/internal/handler"
	"github.com/lixiansheng/fileflow/internal/realtime"
	"github.com/lixiansheng/fileflow/internal/store"
)

func main() {
	cfg := loadConfig()

	if err := run(cfg); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

type config struct {
	ListenAddr     string
	SQLitePath     string
	BootstrapToken string
	AppDomain      string
	RateLimitRPS   float64
	MaxBodyBytes   int64
	SecureCookies  bool
	SessionTTL     time.Duration
	ChallengeTTL   time.Duration
}

func loadConfig() *config {
	return &config{
		ListenAddr:     getEnv("LISTEN_ADDR", ":8080"),
		SQLitePath:     getEnv("SQLITE_PATH", "/data/fileflow.db"),
		BootstrapToken: requireEnv("BOOTSTRAP_TOKEN"),
		AppDomain:      getEnv("APP_DOMAIN", ""),
		RateLimitRPS:   5,
		MaxBodyBytes:   256 * 1024,
		SecureCookies:  getEnv("SECURE_COOKIES", "true") == "true",
		SessionTTL:     12 * time.Hour,
		ChallengeTTL:   60 * time.Second,
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return val
}

func run(cfg *config) error {
	db, err := store.New(cfg.SQLitePath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := validateSecretHash(db); err != nil {
		return err
	}

	challengeStore := auth.NewChallengeStore(cfg.ChallengeTTL)
	defer challengeStore.Stop()

	sessionStore := auth.NewSessionStore(cfg.SessionTTL)
	defer sessionStore.Stop()

	hub := realtime.NewHub()
	go hub.Run()
	defer hub.Stop()

	h := handler.New(handler.Config{
		Store:          db,
		ChallengeStore: challengeStore,
		SessionStore:   sessionStore,
		Hub:            hub,
		BootstrapToken: cfg.BootstrapToken,
		SecureCookies:  cfg.SecureCookies,
		AllowedOrigin:  cfg.AppDomain,
	})

	rateLimiter := handler.NewRateLimiter(cfg.RateLimitRPS, 10)

	routes := handler.Chain(
		h.Routes(),
		handler.LoggingMiddleware,
		rateLimiter.Middleware,
		handler.CORSMiddleware(cfg.AppDomain),
		handler.MaxBytesMiddleware(cfg.MaxBodyBytes),
	)

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      routes,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Server starting on %s", cfg.ListenAddr)
		errCh <- server.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		log.Printf("Received signal %v, shutting down...", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return err
	}

	log.Println("Server stopped gracefully")
	return nil
}

func validateSecretHash(db *store.Store) error {
	_, err := db.GetConfig(store.ConfigKeySecretHash)
	if err == store.ErrConfigNotFound {
		log.Println("WARNING: No secret_hash configured. Run init-secret.sh to set one.")
		return nil
	}
	return err
}
