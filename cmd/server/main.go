package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lixiansheng/fileflow/internal/auth"
	"github.com/lixiansheng/fileflow/internal/handler"
	"github.com/lixiansheng/fileflow/internal/limit"
	"github.com/lixiansheng/fileflow/internal/realtime"
	"github.com/lixiansheng/fileflow/internal/store"
	"golang.org/x/time/rate"
	"strings"
)

func main() {
	cfg := loadConfig()

	if cfg.AppDomain == "" && getEnv("ENV", "") == "prod" {
		log.Fatal("APP_DOMAIN is required in prod")
	}

	if err := run(cfg); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

type config struct {
	ListenAddr      string
	SQLitePath      string
	AppDomain       string
	RateLimitRPS    float64
	MaxBodyBytes    int64
	MaxWSMsgBytes   int
	SecureCookies   bool
	SessionTTL      time.Duration
	ChallengeTTL    time.Duration
	MaxWSConnPerIP  int
	MaxWSConnGlobal int
	BootstrapToken  string
}

func loadConfig() *config {
	return &config{
		ListenAddr:      getEnv("LISTEN_ADDR", ":8080"),
		SQLitePath:      getEnv("SQLITE_PATH", "/data/fileflow.db"),
		AppDomain:       getEnv("APP_DOMAIN", ""),
		RateLimitRPS:    getEnvFloat("RATE_LIMIT_RPS", 5.0),
		MaxBodyBytes:    256 * 1024,
		SecureCookies:   getEnv("SECURE_COOKIES", "true") == "true",
		SessionTTL:      getEnvDurationHours("SESSION_TTL_HOURS", 12*time.Hour, "SESSION_TTL"),
		ChallengeTTL:    60 * time.Second,
		MaxWSMsgBytes:   getEnvInt("MAX_WS_MSG_BYTES", 256*1024),
		MaxWSConnPerIP:  getEnvInt("MAX_WS_CONN_PER_IP", 5),
		MaxWSConnGlobal: getEnvInt("MAX_WS_CONN_GLOBAL", 1000),
		BootstrapToken:  getEnv("BOOTSTRAP_TOKEN", ""),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		var f float64
		if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		var i int
		if _, err := fmt.Sscanf(val, "%d", &i); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func getEnvDurationHours(key string, defaultVal time.Duration, fallbackKey string) time.Duration {
	if val := os.Getenv(key); val != "" {
		var hours float64
		if _, err := fmt.Sscanf(val, "%f", &hours); err == nil {
			return time.Duration(hours * float64(time.Hour))
		}
	}
	if fallbackKey != "" {
		if val := os.Getenv(fallbackKey); val != "" {
			return getEnvDuration(fallbackKey, defaultVal)
		}
	}
	return defaultVal
}

func isDevEnv() bool {
	if os.Getenv("FF_DEV") == "1" {
		return true
	}
	if os.Getenv("APP_ENV") == "dev" {
		return true
	}
	return os.Getenv("ENV") == "dev"
}

func resolveSessionKey(secureCookies bool) (string, error) {
	sessionKey := os.Getenv("SESSION_KEY")
	if sessionKey == "" || sessionKey == "dev-session-key" {
		if secureCookies || !isDevEnv() {
			return "", fmt.Errorf("SESSION_KEY is required")
		}
		if sessionKey == "" {
			log.Println("WARNING: Using default dev-session-key")
		}
		return "dev-session-key", nil
	}
	return sessionKey, nil
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

	// Secret Hash Loading Strategy:
	// 1. Env var APP_SECRET_HASH
	// 2. DB Config (store.ConfigKeySecretHash)
	// 3. Fatal error
	hash := os.Getenv("APP_SECRET_HASH")
	if hash == "" {
		var err error
		hash, err = db.GetConfig(store.ConfigKeySecretHash)
		if err != nil || hash == "" {
			log.Fatal("APP_SECRET_HASH is required")
		}
	}

	sessionKey, err := resolveSessionKey(cfg.SecureCookies)
	if err != nil {
		log.Fatal(err)
	}
	tokenManager := auth.NewTokenManager([]byte(sessionKey))

	proxies := os.Getenv("TRUSTED_PROXY_CIDRS")
	if proxies == "" {
		proxies = os.Getenv("TRUSTED_PROXIES")
	}
	if proxies != "" {
		if err := handler.SetTrustedProxies(strings.Split(proxies, ",")); err != nil {
			log.Fatalf("Invalid trusted proxy list: %v", err)
		}
	}

	connLimiter := limit.NewConnLimiter(cfg.MaxWSConnPerIP, cfg.MaxWSConnGlobal)
	loginLimiter := limit.NewIPLimiter(rate.Limit(cfg.RateLimitRPS), 10)

	challengeStore := auth.NewChallengeStore(cfg.ChallengeTTL)
	defer challengeStore.Stop()

	hub := realtime.NewHub()
	go hub.Run()
	defer hub.Stop()

	h := handler.New(handler.Config{
		Store:          db,
		TokenManager:   tokenManager,
		LoginLimiter:   loginLimiter,
		ConnLimiter:    connLimiter,
		SecretHash:     hash,
		BootstrapToken: cfg.BootstrapToken,
		Hub:            hub,
		SecureCookies:  cfg.SecureCookies,
		SessionTTL:     cfg.SessionTTL,
		ChallengeStore: challengeStore,
		MaxWSMsgBytes:  cfg.MaxWSMsgBytes,
		AllowedOrigin:  cfg.AppDomain,
	})

	rateLimiter := handler.NewRateLimiter(cfg.RateLimitRPS, 10)

	routes := handler.Chain(
		h.Routes(),
		handler.SecurityHeadersMiddleware,
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
