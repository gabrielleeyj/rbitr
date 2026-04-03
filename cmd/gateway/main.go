package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/marketplaceentitlementservice"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	procurement "google.golang.org/api/cloudcommerceprocurement/v1"
	"google.golang.org/api/option"
	servicecontrol "google.golang.org/api/servicecontrol/v2"

	"github.com/gabrielleeyj/rbitr/internal/api/admin"
	"github.com/gabrielleeyj/rbitr/internal/api/public"
	apisetup "github.com/gabrielleeyj/rbitr/internal/api/setup"
	"github.com/gabrielleeyj/rbitr/internal/auth"
	"github.com/gabrielleeyj/rbitr/internal/cache"
	"github.com/gabrielleeyj/rbitr/internal/config"
	"github.com/gabrielleeyj/rbitr/internal/connector"
	"github.com/gabrielleeyj/rbitr/internal/credential"
	"github.com/gabrielleeyj/rbitr/internal/db"
	"github.com/gabrielleeyj/rbitr/internal/license"
	awslicense "github.com/gabrielleeyj/rbitr/internal/license/aws"
	azurelicense "github.com/gabrielleeyj/rbitr/internal/license/azure"
	gcplicense "github.com/gabrielleeyj/rbitr/internal/license/gcp"
	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/notifications"
	"github.com/gabrielleeyj/rbitr/internal/policy"
	"github.com/gabrielleeyj/rbitr/internal/retention"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/ticketing"
)

// version is set at build time via -ldflags.
var version = "dev"

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
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("rbitr-gateway " + version)
		return
	}

	log.Printf("rbitr-gateway %s starting", version)
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

	pubKey, err := license.EmbeddedPublicKey()
	if err != nil {
		log.Fatalf("license: %v", err)
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
	secretProviders := []notifications.SecretProvider{
		notifications.EnvProvider{},
		notifications.FileProvider{},
	}
	secretProviders = appendCloudSecretProviders(secretProviders, &cfg)
	secretResolver := notifications.NewCompositeResolver(secretProviders, secretTTL)
	notificationService := notifications.NewService(st, secretResolver, serviceCooldown, metrics)
	ticketingService := ticketing.NewService(st, secretResolver)
	expiryScheduler := notifications.NewApprovalExpiryScheduler(st, notificationService, time.Minute, approvalExpiryWindow)
	expiryScheduler.Ticketing = ticketingService
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

	licenseProvider, usageReporter, err := initLicenseProvider(&cfg, pubKey, e)
	if err != nil {
		log.Fatalf("license provider: %v", err) //nolint:gocritic // startup init before server starts
	}

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
	provenanceMgr := initProvenanceManager(&cfg)
	credResolver := credential.NewResolver(os.Getenv("VAULT_TOKEN"))

	public.RegisterRoutes(e, &public.Dependencies{
		Store:              st,
		Policy:             policyEval,
		Connector:          restConnector,
		Metrics:            metrics,
		Config:             cfg,
		Notifier:           notificationService,
		ToolCache:          toolCache,
		RiskOverrideCache:  riskOverrideCache,
		SessionManager:     sessionMgr,
		ProvenanceManager:  provenanceMgr,
		TicketingService:   ticketingService,
		LicenseProvider:    licenseProvider,
		UsageReporter:      usageReporter,
		CredentialResolver: credResolver,
	})
	oidcProvider, adminSessionMgr := initSSOComponents(&cfg)

	admin.RegisterRoutes(e, &admin.Dependencies{
		Store:              st,
		Notifications:      notificationService,
		Metrics:            metrics,
		Config:             cfg,
		ToolCache:          toolCache,
		RiskCache:          riskOverrideCache,
		OIDCProvider:       oidcProvider,
		AdminSessionMgr:    adminSessionMgr,
		SecretResolver:     secretResolver,
		TicketingService:   ticketingService,
		LicenseProvider:    licenseProvider,
		CredentialResolver: credResolver,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go expiryScheduler.Start(ctx)
	go auditRetention.Start(ctx)
	go licenseProvider.Start(ctx)
	go usageReporter.Start(ctx)

	sc := echo.StartConfig{
		Address:         cfg.ListenAddr,
		GracefulTimeout: gracefulTimeout,
	}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.Logger.Error("failed to start server", "error", err)
	}
}

func initProvenanceManager(cfg *config.Config) *auth.ProvenanceManager {
	if !cfg.FeatureCrossTenantChain {
		return nil
	}
	pm, err := auth.NewProvenanceManager(cfg.MaxChainDepth)
	if err != nil {
		log.Fatalf("provenance manager init failed: %v", err)
	}
	log.Printf("cross-tenant provenance chain enabled (max_depth=%d)", cfg.MaxChainDepth)
	return pm
}

func initSSOComponents(cfg *config.Config) (*auth.OIDCProvider, *auth.AdminSessionManager) {
	if !cfg.SSOEnabled {
		return nil, nil
	}
	provider := auth.NewOIDCProvider(nil)
	mgr, err := auth.NewAdminSessionManager(auth.AdminSessionTTL)
	if err != nil {
		log.Fatalf("admin session manager init failed: %v", err)
	}
	log.Printf("SSO/OIDC enabled (issuer=%s)", cfg.SSOIssuer)
	return provider, mgr
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

func initLicenseProvider(cfg *config.Config, pubKey ed25519.PublicKey, e *echo.Echo) (license.LicenseProvider, license.UsageReporter, error) {
	switch cfg.LicenseProvider {
	case license.ProviderAWSMarketplace:
		return initAWSLicenseProvider(cfg, e)
	case license.ProviderGCPMarketplace:
		return initGCPLicenseProvider(cfg, e)
	case license.ProviderAzureMarketplace:
		return initAzureLicenseProvider(cfg, e)
	default:
		return license.NewProvider(&license.ProviderConfig{
			Name:    cfg.LicenseProvider,
			PubKey:  pubKey,
			KeyPath: cfg.LicenseKeyPath,
		})
	}
}

func initAWSLicenseProvider(cfg *config.Config, e *echo.Echo) (license.LicenseProvider, license.UsageReporter, error) {
	if cfg.AWSProductCode == "" {
		return nil, nil, errors.New("aws marketplace: RBTR_AWS_PRODUCT_CODE is required")
	}

	opts := []func(*awssdkconfig.LoadOptions) error{}
	if cfg.AWSMarketplaceRegion != "" {
		opts = append(opts, awssdkconfig.WithRegion(cfg.AWSMarketplaceRegion))
	}

	awsCfg, err := awssdkconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("aws marketplace: load AWS config: %w", err)
	}

	entClient := marketplaceentitlementservice.NewFromConfig(awsCfg)
	metClient := marketplacemetering.NewFromConfig(awsCfg)

	provider, err := awslicense.NewProvider(entClient, cfg.AWSProductCode, cfg.AWSCustomerID)
	if err != nil {
		return nil, nil, fmt.Errorf("aws marketplace provider: %w", err)
	}

	reporter := awslicense.NewReporter(metClient, cfg.AWSProductCode, cfg.AWSCustomerID)

	handler := awslicense.NewActivationHandler(metClient, provider, reporter)
	awslicense.RegisterRoutes(e, handler)

	log.Printf("aws marketplace provider enabled (product_code=%s)", cfg.AWSProductCode)
	return provider, reporter, nil
}

func initGCPLicenseProvider(cfg *config.Config, e *echo.Echo) (license.LicenseProvider, license.UsageReporter, error) {
	if cfg.GCPProjectID == "" {
		return nil, nil, errors.New("gcp marketplace: RBTR_GCP_PROJECT_ID is required")
	}
	if cfg.GCPServiceName == "" {
		return nil, nil, errors.New("gcp marketplace: RBTR_GCP_SERVICE_NAME is required")
	}

	procSvc, err := gcpProcurementService(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("gcp marketplace: procurement service: %w", err)
	}

	scSvc, err := gcpServiceControlService(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("gcp marketplace: service control: %w", err)
	}

	procClient := gcplicense.NewSDKProcurementClient(procSvc)
	scClient := gcplicense.NewSDKServiceControlClient(scSvc)

	provider, err := gcplicense.NewProvider(procClient, cfg.GCPProjectID, cfg.GCPServiceName)
	if err != nil {
		return nil, nil, fmt.Errorf("gcp marketplace provider: %w", err)
	}

	reporter := gcplicense.NewReporter(scClient, cfg.GCPServiceName)

	webhookHandler := gcplicense.NewWebhookHandler(provider, procClient)
	gcplicense.RegisterRoutes(e, webhookHandler)

	log.Printf("gcp marketplace provider enabled (project_id=%s, service=%s)", cfg.GCPProjectID, cfg.GCPServiceName)
	return provider, reporter, nil
}

func gcpProcurementService(ctx context.Context) (*procurement.Service, error) {
	return procurement.NewService(ctx, option.WithScopes(procurement.CloudPlatformScope))
}

func gcpServiceControlService(ctx context.Context) (*servicecontrol.Service, error) {
	return servicecontrol.NewService(ctx, option.WithScopes(servicecontrol.ServicecontrolScope))
}

func initAzureLicenseProvider(cfg *config.Config, e *echo.Echo) (license.LicenseProvider, license.UsageReporter, error) {
	if cfg.AzureTenantID == "" {
		return nil, nil, errors.New("azure marketplace: RBTR_AZURE_TENANT_ID is required")
	}
	if cfg.AzureClientID == "" {
		return nil, nil, errors.New("azure marketplace: RBTR_AZURE_CLIENT_ID is required")
	}
	if cfg.AzureClientSecret == "" {
		return nil, nil, errors.New("azure marketplace: RBTR_AZURE_CLIENT_SECRET is required")
	}

	fulfillClient := azurelicense.NewHTTPFulfillmentClient(cfg.AzureTenantID, cfg.AzureClientID, cfg.AzureClientSecret)
	metClient := azurelicense.NewHTTPMeteringClient(fulfillClient)

	provider, err := azurelicense.NewProvider(fulfillClient, "", cfg.AzurePlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("azure marketplace provider: %w", err)
	}

	reporter := azurelicense.NewReporter(metClient, "", cfg.AzurePlanID)

	webhookHandler := azurelicense.NewWebhookHandler(provider, reporter, fulfillClient)
	azurelicense.RegisterRoutes(e, webhookHandler)

	log.Printf("azure marketplace provider enabled (tenant_id=%s)", cfg.AzureTenantID)
	return provider, reporter, nil
}
