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

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gabrielleeyj/rbitr/internal/api/admin"
	"github.com/gabrielleeyj/rbitr/internal/api/public"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/db"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
)

const (
	requestTimeout  = 15 * time.Second
	gracefulTimeout = 10 * time.Second
)

func main() {
	cfg := config.Load()

	dbConn, err := db.Connect(cfg.DatabaseURL)
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
	}, 5*time.Minute)
	notificationService := notifications.NewService(st, secretResolver, 10*time.Minute, metrics)
	expiryScheduler := notifications.NewApprovalExpiryScheduler(st, notificationService, time.Minute, 5*time.Minute)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.ContextTimeoutWithConfig(middleware.ContextTimeoutConfig{
		Timeout: requestTimeout,
	}))
	e.Use(telemetry.RequestLogger())

	e.GET("/healthz", func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	public.RegisterRoutes(e, &public.Dependencies{
		Store:     st,
		Policy:    policyEval,
		Connector: restConnector,
		Metrics:   metrics,
		Config:    cfg,
		Notifier:  notificationService,
	})
	admin.RegisterRoutes(e, &admin.Dependencies{
		Store:         st,
		Notifications: notificationService,
		Metrics:       metrics,
		Config:        cfg,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go expiryScheduler.Start(ctx)

	sc := echo.StartConfig{
		Address:         cfg.ListenAddr,
		GracefulTimeout: gracefulTimeout,
	}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.Logger.Error("failed to start server", "error", err)
	}
}
