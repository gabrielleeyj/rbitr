package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gabrielleeyj/rbitr/internal/api/admin"
	"github.com/gabrielleeyj/rbitr/internal/api/public"
	apisetup "github.com/gabrielleeyj/rbitr/internal/api/setup"
	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/cache"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/db"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/retention"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

const (
	requestTimeout         = 15 * time.Second
	gracefulTimeout        = 10 * time.Second
	cacheTTL               = 30 * time.Second
	secretTTL              = 5 * time.Minute
	serviceCooldown        = 10 * time.Minute
	approvalExpiryWindow   = 5 * time.Minute
	auditRetentionInterval = 24 * time.Hour
)

func main() {
	cfg := config.Load()

	setupToken, err := apisetup.ResolveSetupToken(cfg.SetupToken, cfg.SetupTokenFile)
	if err != nil {
		log.Fatalf("resolve setup token failed: %v", err)
	}
	if cfg.SetupTokenRequired && strings.TrimSpace(setupToken) == "" {
		log.Fatal("RBTR_SETUP_TOKEN_REQUIRED=true requires RBTR_SETUP_TOKEN or RBTR_SETUP_TOKEN_FILE")
	}
	allowedCIDRs, err := apisetup.ParseAllowedCIDRs(cfg.SetupAllowedCIDRs)
	if err != nil {
		log.Fatalf("invalid RBTR_SETUP_ALLOWED_CIDRS: %v", err)
	}

	dbConn, err := db.Connect(cfg.DatabaseURL, db.PoolConfig{
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
		ConnMaxIdleTime: cfg.DBConnMaxIdleTime,
	})
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer dbConn.Close()

	st := store.New(dbConn)
	metrics := telemetry.NewMetrics()
	policyEval := policy.NewEvaluator(st)
	restConnector := connector.NewREST(cfg.ResponseLimit)
	secretResolver := notifications.NewCompositeResolver([]notifications.SecretProvider{
		notifications.EnvProvider{},
		notifications.FileProvider{},
	}, secretTTL)
	notificationService := notifications.NewService(st, secretResolver, serviceCooldown, metrics)
	expiryScheduler := notifications.NewApprovalExpiryScheduler(st, notificationService, time.Minute, approvalExpiryWindow)
	auditRetention := retention.NewAuditRetentionScheduler(st, auditRetentionInterval)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: requestTimeout,
	}))
	e.Use(telemetry.RequestLogger())

	e.GET("/healthz", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/readyz", func(c *echo.Context) error {
		ctx := c.Request().Context()
		pingErr := dbConn.PingContext(ctx)
		if pingErr != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not ready", "reason": "database unreachable"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	toolCache := cache.New[models.Tool](cacheTTL)
	riskOverrideCache := cache.New[string](cacheTTL)

	setupService := apisetup.NewService(dbConn, apisetup.Options{
		DevAutoTools:        cfg.DevAutoTools,
		DevMockInternalURL:  cfg.DevMockInternalURL,
		DevJiraURL:          cfg.DevJiraURL,
		IdempotencyRequired: cfg.SetupTokenRequired,
		Metrics:             metrics,
	})

	apisetup.RegisterRoutes(e, &apisetup.Dependencies{
		Service: setupService,
		AccessPolicy: apisetup.AccessPolicy{
			TokenRequired: cfg.SetupTokenRequired,
			Token:         setupToken,
			AllowedCIDRs:  allowedCIDRs,
		},
	})

	sessionMgr := initSessionManager(&cfg)

	public.RegisterRoutes(e, &public.Dependencies{
		Store:             st,
		Policy:            policyEval,
		Connector:         restConnector,
		Metrics:           metrics,
		Config:            cfg,
		Notifier:          notificationService,
		ToolCache:         toolCache,
		RiskOverrideCache: riskOverrideCache,
		SessionManager:    sessionMgr,
	})
	admin.RegisterRoutes(e, &admin.Dependencies{
		Store:         st,
		Notifications: notificationService,
		Metrics:       metrics,
		Config:        cfg,
		ToolCache:     toolCache,
		RiskCache:     riskOverrideCache,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go expiryScheduler.Start(ctx)
	go auditRetention.Start(ctx)

	sc := echo.StartConfig{
		Address:         cfg.ListenAddr,
		GracefulTimeout: gracefulTimeout,
	}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.Logger.Error("failed to start server", "error", err)
	}
}

func initSessionManager(cfg *config.Config) *auth.SessionManager {
	if !cfg.FeatureSessionTokens {
		return nil
	}
	sm, err := auth.NewSessionManager(cfg.SessionTokenTTL)
	if err != nil {
		log.Fatalf("session manager init failed: %v", err)
	}
	log.Printf("session tokens enabled (TTL=%s)", cfg.SessionTokenTTL)
	return sm
}
