package setup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/telemetry"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrSetupComplete       = errors.New("setup already complete")
	ErrSetupInProgress     = errors.New("setup already in progress")
	ErrSchemaNotReady      = errors.New("schema not ready")
	ErrInvalidRequest      = errors.New("invalid setup request")
	ErrIdempotencyRequired = errors.New("idempotency key required")
	ErrIdempotencyConflict = errors.New("idempotency key payload mismatch")

	tenantIDPattern   = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{3,64}$`)
	tenantNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 ._\-]{1,119}$`)
)

const (
	adminKeyIDPattern      = "admin_%s"
	defaultPolicyNotes     = "Initialized by setup workflow"
	setupBootstrapKey      = "bootstrap_complete"
	setupAdminWriteLockKey = "admin_write_lock"

	setupStateNotStarted = "not_started"
	setupStateInProgress = "in_progress"
	setupStateCompleted  = "completed"
	setupStateFailed     = "failed"

	outcomeFailed              = "failed"
	outcomeInvalidRequest      = "invalid_request"
	outcomeMissingIdempotency  = "missing_idempotency"
	outcomeIdempotencyConflict = "idempotency_conflict"
	outcomeReplay              = "replay"
	outcomeSchemaNotReady      = "schema_not_ready"
	outcomeInProgress          = "in_progress"
	outcomeAlreadyComplete     = "already_complete"
	outcomeSuccess             = "success"

	setupInitializeLockKey int64 = 1257826389

	minSecretCharacterClasses = 2
	minSecretLength           = 16
	minSecretUniqueRunes      = 8

	defaultPolicyVersion = "p_v1"

	defaultApprovalTTLSeconds = "900"
	defaultAuditRetentionDays = "365"
	defaultRateLimitPerMinute = "60"
	defaultRateLimitPerDay    = "10000"
	defaultRateLimitScope     = "tenant_agent_tool"

	defaultBooleanFalse = "false"

	adminKeyPrefix      = "rbtr_admin_"
	devToolMockInternal = "mock_internal"
	devToolJira         = "jira"

	defaultDevMockInternalURL = "http://localhost:8090"
	defaultDevJiraURL         = "http://localhost:8081"

	devMockInternalAuthType  = "api_key"
	devMockInternalAuthValue = "mock_internal_key"
	devJiraAuthType          = "bearer"
	devJiraAuthValue         = "jira_token"

	defaultPolicyModule = `package rbitr.policy

import rego.v1

decision_obj(decision, risk, rule_id, priority, code, message) := {
	"version": "2026-01-20",
	"decision": decision,
	"risk": risk,
	"rule": {"id": rule_id, "priority": priority},
	"reasons": [{"code": code, "message": message}],
	"constraints": {},
	"tags": []
}

allow_actions := {
	"TICKET.CREATE",
	"TICKET.COMMENT",
	"TICKET.UPDATE",
	"CRM.READ",
	"DATA.READ",
	"DATA.QUERY"
}

require_approval_actions := {
	"PAYMENT.REFUND",
	"ACCESS.ROLE_CHANGE"
}

deny_actions := {
	"DATA.EXPORT",
	"DATA.BULK_EXPORT",
	"ACCESS.GRANT",
	"DATA.DELETE",
	"CRM.DELETE"
}

decision := decision_obj("DENY", input.action_risk, "rule_deny_sensitive_v1", 100, "DENY_SENSITIVE", "Policy: deny sensitive action") if {
	input.action_type
	deny_actions[input.action_type]
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_require_approval_v1", 50, "APPROVAL_REQUIRED", "Policy: approval required") if {
	input.action_type
	require_approval_actions[input.action_type]
} else := decision_obj("ALLOW", input.action_risk, "rule_allow_basic_actions_v1", 10, "ALLOW_BASIC", "Policy: allow basic actions") if {
	input.action_type
	allow_actions[input.action_type]
} else := decision_obj("ALLOW", input.action_risk, "rule_allow_mcp_tools", 15, "ALLOW_MCP", "Policy: allow MCP tool calls") if {
	input.action_type
	startswith(input.action_type, "MCP.")
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_high_risk_unknown", 80, "HIGH_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
	input.action_risk == "HIGH"
} else := decision_obj("REQUIRE_APPROVAL", input.action_risk, "rule_critical_risk_unknown", 80, "CRITICAL_RISK_UNKNOWN", "Policy: approval required for high/critical risk") if {
	input.action_risk == "CRITICAL"
} else := decision_obj("DENY", "MEDIUM", "rule_default_deny", 100, "DEFAULT_DENY", "Default deny: no matching rule or missing required fields")
`
)

type Options struct {
	DevAutoTools        bool
	DevMockInternalURL  string
	DevJiraURL          string
	IdempotencyRequired bool
	Metrics             *telemetry.Metrics
}

type devToolSeed struct {
	toolID    string
	baseURL   string
	authType  string
	authValue string
}

type StatusResponse struct {
	SetupRequired           bool   `json:"setup_required"`
	BootstrapComplete       bool   `json:"bootstrap_complete"`
	DatabaseReachable       bool   `json:"database_reachable"`
	SchemaReady             bool   `json:"schema_ready"`
	AdminKeyCount           int    `json:"admin_key_count"`
	TenantCount             int    `json:"tenant_count"`
	SetupState              string `json:"setup_state"`
	LastError               string `json:"last_error,omitempty"`
	InitializeAllowed       bool   `json:"initialize_allowed"`
	InitializeTokenRequired bool   `json:"initialize_token_required"`
	IdempotencyRequired     bool   `json:"idempotency_required"`
}

type InitializeRequest struct {
	TenantName string `json:"tenant_name"`
	TenantID   string `json:"tenant_id"`
	AdminKey   string `json:"admin_key"`
	TenantKey  string `json:"tenant_key"`

	IdempotencyKey        string `json:"-"`
	SetupTokenFingerprint string `json:"-"`
	ClientIP              string `json:"-"`
	UserAgent             string `json:"-"`
	RequestID             string `json:"-"`
}

type InitializeResponse struct {
	BootstrapComplete bool   `json:"bootstrap_complete"`
	TenantID          string `json:"tenant_id"`
	TenantName        string `json:"tenant_name"`
	TenantKeyID       string `json:"tenant_key_id"`
	TenantKey         string `json:"tenant_key"`
	TenantKeyCreated  bool   `json:"tenant_key_created"`
	AdminKeyID        string `json:"admin_key_id"`
	AdminKey          string `json:"admin_key"`
	AdminKeyCreated   bool   `json:"admin_key_created"`
	PolicyVersion     string `json:"policy_version"`
}

type Service interface {
	Status(ctx context.Context) (StatusResponse, error)
	Initialize(ctx context.Context, req *InitializeRequest) (InitializeResponse, error)
}

type dbService struct {
	db                  *sql.DB
	devAutoTools        bool
	devMockInternalURL  string
	devJiraURL          string
	idempotencyRequired bool
	metrics             *telemetry.Metrics
	auditStore          store.StoreAPI
}

type normalizedInitializeRequest struct {
	tenantName       string
	tenantID         string
	adminKey         string
	tenantKey        string
	tenantKeyHash    string
	tenantKeyPrefix  string
	adminKeyCreated  bool
	tenantKeyCreated bool
}

type setupStateRecord struct {
	State   string
	LastErr string
}

type idempotencyReplay struct {
	PayloadHash string
	ResponseRaw []byte
}

type requestValidationError struct {
	Fields map[string]string
}

func (e *requestValidationError) Error() string {
	if len(e.Fields) == 0 {
		return ErrInvalidRequest.Error()
	}
	parts := make([]string, 0, len(e.Fields))
	for field, msg := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
	}
	return fmt.Sprintf("%s: %s", ErrInvalidRequest.Error(), strings.Join(parts, ", "))
}

func (e *requestValidationError) Unwrap() error {
	return ErrInvalidRequest
}

func fieldErrorsFromError(err error) map[string]string {
	var validationErr *requestValidationError
	if !errors.As(err, &validationErr) || len(validationErr.Fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(validationErr.Fields))
	for key, value := range validationErr.Fields {
		out[key] = value
	}
	return out
}

func NewService(db *sql.DB, opts ...Options) Service {
	cfg := Options{}
	if len(opts) > 0 {
		cfg = opts[0]
	}
	return &dbService{
		db:                  db,
		devAutoTools:        cfg.DevAutoTools,
		devMockInternalURL:  coalesceDefault(strings.TrimSpace(cfg.DevMockInternalURL), defaultDevMockInternalURL),
		devJiraURL:          coalesceDefault(strings.TrimSpace(cfg.DevJiraURL), defaultDevJiraURL),
		idempotencyRequired: cfg.IdempotencyRequired,
		metrics:             cfg.Metrics,
		auditStore:          store.New(db),
	}
}

func (s *dbService) Status(ctx context.Context) (StatusResponse, error) {
	status := StatusResponse{
		DatabaseReachable:       true,
		SetupState:              setupStateNotStarted,
		InitializeTokenRequired: s.idempotencyRequired,
		IdempotencyRequired:     s.idempotencyRequired,
	}

	schemaReady, err := s.schemaReady(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	status.SchemaReady = schemaReady

	if !schemaReady {
		status.SetupRequired = true
		status.InitializeAllowed = true
		s.observeSetupState(status.SetupState)
		return status, nil
	}

	bootstrapComplete, err := getBootstrapComplete(ctx, s.db)
	if err != nil {
		return StatusResponse{}, err
	}
	status.BootstrapComplete = bootstrapComplete
	status.SetupRequired = !bootstrapComplete
	status.InitializeAllowed = !bootstrapComplete

	stateRecord, err := getSetupState(ctx, s.db)
	if err != nil {
		return StatusResponse{}, err
	}
	if strings.TrimSpace(stateRecord.State) != "" {
		status.SetupState = stateRecord.State
	}
	status.LastError = stateRecord.LastErr
	if bootstrapComplete {
		status.SetupState = setupStateCompleted
		status.LastError = ""
	}

	adminCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM rbitr.admin_keys`)
	if err != nil {
		return StatusResponse{}, err
	}
	tenantCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM rbitr.tenants`)
	if err != nil {
		return StatusResponse{}, err
	}
	status.AdminKeyCount = adminCount
	status.TenantCount = tenantCount
	s.observeSetupState(status.SetupState)

	return status, nil
}

func (s *dbService) Initialize(ctx context.Context, req *InitializeRequest) (_ InitializeResponse, retErr error) {
	if req == nil {
		return InitializeResponse{}, validationError("request", "request is required")
	}
	started := time.Now().UTC()
	outcome := outcomeFailed
	auditStarted := false
	auditTenantID := strings.TrimSpace(req.TenantID)
	log.Printf(
		"event=setup.initialize.start request_id=%s client_ip=%s token_fingerprint=%s idempotency_key_present=%t",
		req.RequestID,
		req.ClientIP,
		req.SetupTokenFingerprint,
		strings.TrimSpace(req.IdempotencyKey) != "",
	)
	defer func() {
		if retErr != nil &&
			auditStarted &&
			!errors.Is(retErr, ErrSetupComplete) &&
			!errors.Is(retErr, ErrSetupInProgress) &&
			!errors.Is(retErr, ErrIdempotencyConflict) {
			s.emitSetupAudit(ctx, req, auditTenantID, "SETUP.FAILED", "SETUP", "bootstrap", map[string]any{
				"error":      retErr.Error(),
				"request_id": req.RequestID,
				"client_ip":  req.ClientIP,
			})
		}
		durationMs := float64(time.Since(started).Milliseconds())
		if s.metrics != nil {
			if s.metrics.SetupAttemptsTotal != nil {
				s.metrics.SetupAttemptsTotal.WithLabelValues(outcome).Inc()
			}
			if s.metrics.SetupDurationMs != nil {
				s.metrics.SetupDurationMs.Observe(durationMs)
			}
		}
		if retErr != nil {
			log.Printf(
				"event=setup.initialize.failed request_id=%s client_ip=%s token_fingerprint=%s outcome=%s duration_ms=%.0f error=%q",
				req.RequestID,
				req.ClientIP,
				req.SetupTokenFingerprint,
				outcome,
				durationMs,
				retErr.Error(),
			)
			return
		}
		log.Printf(
			"event=setup.initialize.success request_id=%s client_ip=%s token_fingerprint=%s outcome=%s duration_ms=%.0f",
			req.RequestID,
			req.ClientIP,
			req.SetupTokenFingerprint,
			outcome,
			durationMs,
		)
	}()

	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if s.idempotencyRequired && idempotencyKey == "" {
		outcome = outcomeMissingIdempotency
		return InitializeResponse{}, ErrIdempotencyRequired
	}

	payloadHash := ""
	if idempotencyKey != "" {
		payloadHash = initializePayloadHash(req)
		replayResp, replayFound, replayErr := checkInitializeReplay(ctx, s.db, idempotencyKey, payloadHash)
		if replayErr != nil {
			if errors.Is(replayErr, ErrIdempotencyConflict) {
				outcome = outcomeIdempotencyConflict
			}
			return InitializeResponse{}, replayErr
		}
		if replayFound {
			outcome = outcomeReplay
			return replayResp, nil
		}
	}

	normalized, err := normalizeInitializeRequest(req)
	if err != nil {
		_ = s.writeSetupState(
			ctx,
			setupStateFailed,
			err.Error(),
			req,
			ptrTime(started),
			nil,
		)
		s.observeSetupState(setupStateFailed)
		outcome = outcomeInvalidRequest
		return InitializeResponse{}, err
	}
	auditTenantID = normalized.tenantID

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	schemaReady, err := schemaReadyWithQuerier(ctx, tx)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}
	if !schemaReady {
		_ = s.writeSetupState(ctx, setupStateFailed, ErrSchemaNotReady.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		outcome = outcomeSchemaNotReady
		return InitializeResponse{}, ErrSchemaNotReady
	}

	locked, err := trySetupInitializeLock(ctx, tx)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}
	if !locked {
		outcome = outcomeInProgress
		return InitializeResponse{}, ErrSetupInProgress
	}

	if idempotencyKey != "" {
		replayResp, replayFound, replayErr := checkInitializeReplay(ctx, tx, idempotencyKey, payloadHash)
		if replayErr != nil {
			_ = s.writeSetupState(ctx, setupStateFailed, replayErr.Error(), req, ptrTime(started), nil)
			s.observeSetupState(setupStateFailed)
			if errors.Is(replayErr, ErrIdempotencyConflict) {
				outcome = outcomeIdempotencyConflict
			}
			return InitializeResponse{}, replayErr
		}
		if replayFound {
			outcome = outcomeReplay
			return replayResp, nil
		}
	}

	writeStateErr := s.writeSetupState(
		ctx,
		setupStateInProgress,
		"",
		req,
		ptrTime(started),
		nil,
	)
	if writeStateErr != nil {
		log.Printf("setup state write failed (in_progress): %v", writeStateErr)
	} else {
		s.observeSetupState(setupStateInProgress)
	}
	s.emitSetupAudit(ctx, req, "", "SETUP.START", "SETUP", "bootstrap", map[string]any{
		"request_id": req.RequestID,
		"client_ip":  req.ClientIP,
	})
	auditStarted = true

	bootstrapComplete, err := getBootstrapComplete(ctx, tx)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}
	if bootstrapComplete {
		completed := time.Now().UTC()
		_ = s.writeSetupState(
			ctx,
			setupStateCompleted,
			"",
			req,
			ptrTime(started),
			&completed,
		)
		s.observeSetupState(setupStateCompleted)
		outcome = outcomeAlreadyComplete
		return InitializeResponse{}, ErrSetupComplete
	}

	tenantIDExists, err := tenantIDExists(ctx, tx, normalized.tenantID)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}
	if tenantIDExists {
		validationErr := validationError("tenant_id", "tenant_id already exists")
		_ = s.writeSetupState(ctx, setupStateFailed, validationErr.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		outcome = outcomeInvalidRequest
		return InitializeResponse{}, validationErr
	}

	tenantNameExists, err := tenantNameExists(ctx, tx, normalized.tenantName)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}
	if tenantNameExists {
		validationErr := validationError("tenant_name", "tenant_name already exists")
		_ = s.writeSetupState(ctx, setupStateFailed, validationErr.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		outcome = outcomeInvalidRequest
		return InitializeResponse{}, validationErr
	}

	now := time.Now().UTC()
	tenantKeyID := uuid.NewString()
	adminKeyID := fmt.Sprintf(adminKeyIDPattern, uuid.NewString()[:8])

	_, err = tx.ExecContext(ctx,
		`INSERT INTO rbitr.tenants (tenant_id, name, enabled, created_at) VALUES ($1, $2, true, $3)`,
		normalized.tenantID, normalized.tenantName, now,
	)
	if err != nil {
		ret := normalizeWriteError(err, "tenant_id")
		_ = s.writeSetupState(ctx, setupStateFailed, ret.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, ret
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO rbitr.tenant_keys (key_id, tenant_id, key_hash, key_prefix, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantKeyID, normalized.tenantID, normalized.tenantKeyHash, normalized.tenantKeyPrefix, now,
	)
	if err != nil {
		ret := normalizeWriteError(err, "tenant_key")
		_ = s.writeSetupState(ctx, setupStateFailed, ret.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, ret
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO rbitr.admin_keys (admin_key_id, key_hash, scopes, created_at)
		 VALUES ($1, $2, $3, $4)`,
		adminKeyID, utils.HashAdminKey(normalized.adminKey), store.StringArray{"admin:read", "admin:write"}, now,
	)
	if err != nil {
		ret := normalizeWriteError(err, "admin_key")
		_ = s.writeSetupState(ctx, setupStateFailed, ret.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, ret
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO rbitr.policy_versions (tenant_id, policy_version, rego_module, created_at, created_by, notes)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		normalized.tenantID, defaultPolicyVersion, defaultPolicyModule, now, "setup_wizard", defaultPolicyNotes,
	)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO rbitr.tenant_config (tenant_id, active_policy_version, created_at, updated_at, version)
		 VALUES ($1, $2, $3, $4, 1)
		 ON CONFLICT (tenant_id) DO UPDATE
		 SET active_policy_version = $2, updated_at = $4, version = rbitr.tenant_config.version + 1`,
		normalized.tenantID, defaultPolicyVersion, now, now,
	)
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}

	if s.devAutoTools {
		err = s.insertDevTools(ctx, tx, normalized.tenantID, now)
		if err != nil {
			_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
			s.observeSetupState(setupStateFailed)
			return InitializeResponse{}, err
		}
	}

	settingWrites := []struct {
		key   string
		value string
	}{
		{key: setupBootstrapKey, value: "true"},
		{key: setupAdminWriteLockKey, value: defaultBooleanFalse},
		{key: "default_approval_ttl_seconds", value: defaultApprovalTTLSeconds},
		{key: "audit_retention_days", value: defaultAuditRetentionDays},
		{key: "disable_x_tenant_key", value: defaultBooleanFalse},
		{key: "feature_rate_limiting", value: defaultBooleanFalse},
		{key: "feature_arg_constraints", value: defaultBooleanFalse},
		{key: "default_rate_limit_per_minute", value: defaultRateLimitPerMinute},
		{key: "default_rate_limit_per_day", value: defaultRateLimitPerDay},
		{key: "default_rate_limit_scope", value: defaultRateLimitScope},
	}
	for _, setting := range settingWrites {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO rbitr.system_settings (key, value, updated_at)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`,
			setting.key, setting.value, now,
		)
		if err != nil {
			_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
			s.observeSetupState(setupStateFailed)
			return InitializeResponse{}, err
		}
	}

	response := InitializeResponse{
		BootstrapComplete: true,
		TenantID:          normalized.tenantID,
		TenantName:        normalized.tenantName,
		TenantKeyID:       tenantKeyID,
		TenantKey:         normalized.tenantKey,
		TenantKeyCreated:  normalized.tenantKeyCreated,
		AdminKeyID:        adminKeyID,
		AdminKey:          normalized.adminKey,
		AdminKeyCreated:   normalized.adminKeyCreated,
		PolicyVersion:     defaultPolicyVersion,
	}

	if idempotencyKey != "" {
		responseJSON, marshalErr := json.Marshal(response)
		if marshalErr != nil {
			_ = s.writeSetupState(ctx, setupStateFailed, marshalErr.Error(), req, ptrTime(started), nil)
			s.observeSetupState(setupStateFailed)
			return InitializeResponse{}, marshalErr
		}
		err = storeIdempotencyReplay(
			ctx,
			tx,
			idempotencyKey,
			payloadHash,
			responseJSON,
			req.SetupTokenFingerprint,
			req.ClientIP,
			now,
		)
		if err != nil {
			_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
			s.observeSetupState(setupStateFailed)
			return InitializeResponse{}, err
		}
	}

	err = tx.Commit()
	if err != nil {
		_ = s.writeSetupState(ctx, setupStateFailed, err.Error(), req, ptrTime(started), nil)
		s.observeSetupState(setupStateFailed)
		return InitializeResponse{}, err
	}

	completedAt := time.Now().UTC()
	_ = s.writeSetupState(
		ctx,
		setupStateCompleted,
		"",
		req,
		ptrTime(started),
		&completedAt,
	)
	s.observeSetupState(setupStateCompleted)
	s.emitSetupAudit(ctx, req, normalized.tenantID, "SETUP.SUCCEEDED", "SETUP", "bootstrap", map[string]any{
		"tenant_id":      normalized.tenantID,
		"policy_version": defaultPolicyVersion,
		"duration_ms":    time.Since(started).Milliseconds(),
	})
	s.emitSetupAudit(ctx, req, normalized.tenantID, "SETUP.KEYS.CREATED", "TENANT.KEY", tenantKeyID, map[string]any{
		"tenant_key_id": tenantKeyID,
		"admin_key_id":  adminKeyID,
	})
	outcome = outcomeSuccess
	return response, nil
}

func (s *dbService) emitSetupAudit(
	ctx context.Context,
	req *InitializeRequest,
	tenantID, action, resourceType, resourceID string,
	after map[string]any,
) {
	if s.auditStore == nil {
		return
	}
	afterJSON, err := marshalAuditJSON(after)
	if err != nil {
		log.Printf("setup audit marshal after failed: %v", err)
		return
	}

	var actorFingerprint string
	if req.SetupTokenFingerprint != "" {
		actorFingerprint = strings.TrimSpace(req.SetupTokenFingerprint)
	}
	if actorFingerprint == "" {
		actorFingerprint = "setup_token"
	}
	event := &models.AdminAuditEvent{
		AuditEventID: "ae_" + uuid.NewString(),
		TenantID:     tenantID,
		ActorType:    "setup_token",
		ActorID:      actorFingerprint,
		ActorDisplay: actorFingerprint,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		After:        afterJSON,
		RequestID:    req.RequestID,
		IP:           req.ClientIP,
		UserAgent:    req.UserAgent,
		CreatedAt:    time.Now().UTC(),
	}
	insertErr := s.auditStore.InsertAuditEvent(ctx, event)
	if insertErr != nil {
		log.Printf("setup audit insert failed: %v", insertErr)
	}
}

func marshalAuditJSON(payload map[string]any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *dbService) writeSetupState(
	ctx context.Context,
	state, lastError string,
	req *InitializeRequest,
	startedAt, completedAt *time.Time,
) error {
	tokenFingerprint := ""
	clientIP := ""
	requestID := ""
	if req != nil {
		tokenFingerprint = req.SetupTokenFingerprint
		clientIP = req.ClientIP
		requestID = req.RequestID
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO rbitr.setup_state (
			singleton, state, last_error, actor_token_fingerprint, actor_ip, last_request_id, started_at, completed_at, updated_at
		) VALUES (TRUE, $1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (singleton) DO UPDATE
		SET state = EXCLUDED.state,
			last_error = EXCLUDED.last_error,
			actor_token_fingerprint = EXCLUDED.actor_token_fingerprint,
			actor_ip = EXCLUDED.actor_ip,
			last_request_id = EXCLUDED.last_request_id,
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			updated_at = EXCLUDED.updated_at`,
		state,
		nullableString(lastError),
		nullableString(tokenFingerprint),
		nullableString(clientIP),
		nullableString(requestID),
		startedAt,
		completedAt,
		time.Now().UTC(),
	)
	return err
}

func (s *dbService) observeSetupState(state string) {
	if s.metrics == nil || s.metrics.SetupState == nil {
		return
	}
	states := []string{setupStateNotStarted, setupStateInProgress, setupStateCompleted, setupStateFailed}
	for _, candidate := range states {
		value := 0.0
		if candidate == state {
			value = 1.0
		}
		s.metrics.SetupState.WithLabelValues(candidate).Set(value)
	}
}

func (s *dbService) insertDevTools(ctx context.Context, tx *sql.Tx, tenantID string, createdAt time.Time) error {
	devToolSeeds := []devToolSeed{
		{
			toolID:    devToolMockInternal,
			baseURL:   s.devMockInternalURL,
			authType:  devMockInternalAuthType,
			authValue: devMockInternalAuthValue,
		},
		{
			toolID:    devToolJira,
			baseURL:   s.devJiraURL,
			authType:  devJiraAuthType,
			authValue: devJiraAuthValue,
		},
	}
	for _, tool := range devToolSeeds {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rbitr.tools (tool_id, tenant_id, base_url, auth_type, auth_value, source, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (tool_id, tenant_id) DO UPDATE
			 SET base_url = EXCLUDED.base_url,
			     auth_type = EXCLUDED.auth_type,
			     auth_value = EXCLUDED.auth_value,
			     source = EXCLUDED.source`,
			tool.toolID,
			tenantID,
			tool.baseURL,
			tool.authType,
			tool.authValue,
			"dev_seed",
			createdAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func coalesceDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeInitializeRequest(req *InitializeRequest) (normalizedInitializeRequest, error) {
	fields := map[string]string{}
	if req == nil {
		return normalizedInitializeRequest{}, validationError("request", "request is required")
	}

	tenantName := strings.TrimSpace(req.TenantName)
	switch {
	case tenantName == "":
		fields["tenant_name"] = "tenant_name is required"
	case len(tenantName) > 120: //nolint:mnd // field length bound.
		fields["tenant_name"] = "tenant_name must be 120 characters or fewer"
	case !tenantNamePattern.MatchString(tenantName):
		fields["tenant_name"] = "tenant_name must start with alphanumeric and use alphanumeric, space, dot, underscore, or hyphen"
	}

	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = "t_" + uuid.NewString()[:8]
	}
	if !tenantIDPattern.MatchString(tenantID) {
		fields["tenant_id"] = "tenant_id must match " + tenantIDPattern.String()
	}

	adminKey := strings.TrimSpace(req.AdminKey)
	adminKeyCreated := false
	if adminKey == "" {
		generated, err := generateSecret(adminKeyPrefix)
		if err != nil {
			return normalizedInitializeRequest{}, err
		}
		adminKey = generated
		adminKeyCreated = true
	} else if err := validateUserSuppliedSecret("admin_key", adminKey); err != nil {
		fields["admin_key"] = err.Error()
	}

	tenantKey := strings.TrimSpace(req.TenantKey)
	tenantKeyCreated := false
	tenantKeyHash := ""
	tenantKeyPrefix := ""
	if tenantKey == "" {
		generated, hash, prefix, err := utils.GenerateAPIKey()
		if err != nil {
			return normalizedInitializeRequest{}, err
		}
		tenantKey = generated
		tenantKeyHash = hash
		tenantKeyPrefix = prefix
		tenantKeyCreated = true
	} else {
		if err := validateUserSuppliedSecret("tenant_key", tenantKey); err != nil {
			fields["tenant_key"] = err.Error()
		}
		tenantKeyHash = utils.HashTenantKey(tenantKey)
		tenantKeyPrefix = keyPrefix(tenantKey)
	}

	if len(fields) > 0 {
		return normalizedInitializeRequest{}, &requestValidationError{Fields: fields}
	}

	return normalizedInitializeRequest{
		tenantName:       tenantName,
		tenantID:         tenantID,
		adminKey:         adminKey,
		tenantKey:        tenantKey,
		tenantKeyHash:    tenantKeyHash,
		tenantKeyPrefix:  tenantKeyPrefix,
		adminKeyCreated:  adminKeyCreated,
		tenantKeyCreated: tenantKeyCreated,
	}, nil
}

func validateUserSuppliedSecret(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.ContainsAny(value, "\t\n\r ") {
		return fmt.Errorf("%s must not contain whitespace", field)
	}
	if len(trimmed) < minSecretLength {
		return fmt.Errorf("%s must be at least 16 characters", field)
	}
	unique := map[rune]struct{}{}
	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSymbol := false
	for _, r := range trimmed {
		unique[r] = struct{}{}
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	if len(unique) < minSecretUniqueRunes {
		return fmt.Errorf("%s has insufficient entropy", field)
	}
	classCount := 0
	if hasLower {
		classCount++
	}
	if hasUpper {
		classCount++
	}
	if hasDigit {
		classCount++
	}
	if hasSymbol {
		classCount++
	}
	if classCount < minSecretCharacterClasses {
		return fmt.Errorf("%s must include at least two character classes", field)
	}
	return nil
}

func validationError(field, message string) error {
	return &requestValidationError{
		Fields: map[string]string{
			field: message,
		},
	}
}

type queryRowScanner interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type execContext interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type setupQuerier interface {
	queryRowScanner
}

type setupExecQuerier interface {
	queryRowScanner
	execContext
}

func (s *dbService) schemaReady(ctx context.Context) (bool, error) {
	return schemaReadyWithQuerier(ctx, s.db)
}

func schemaReadyWithQuerier(ctx context.Context, q setupQuerier) (bool, error) {
	row := q.QueryRowContext(ctx, `SELECT
		to_regclass('rbitr.tenants') IS NOT NULL,
		to_regclass('rbitr.tenant_keys') IS NOT NULL,
		to_regclass('rbitr.admin_keys') IS NOT NULL,
		to_regclass('rbitr.policy_versions') IS NOT NULL,
		to_regclass('rbitr.tenant_config') IS NOT NULL,
		to_regclass('rbitr.system_settings') IS NOT NULL,
		to_regclass('rbitr.setup_state') IS NOT NULL,
		to_regclass('rbitr.setup_initialize_idempotency') IS NOT NULL`)

	var hasTenants, hasTenantKeys, hasAdminKeys, hasPolicyVersions, hasTenantConfig, hasSystemSettings bool
	var hasSetupState, hasSetupIdempotency bool
	if err := row.Scan(
		&hasTenants,
		&hasTenantKeys,
		&hasAdminKeys,
		&hasPolicyVersions,
		&hasTenantConfig,
		&hasSystemSettings,
		&hasSetupState,
		&hasSetupIdempotency,
	); err != nil {
		return false, err
	}

	return hasTenants &&
		hasTenantKeys &&
		hasAdminKeys &&
		hasPolicyVersions &&
		hasTenantConfig &&
		hasSystemSettings &&
		hasSetupState &&
		hasSetupIdempotency, nil
}

func getBootstrapComplete(ctx context.Context, q setupQuerier) (bool, error) {
	row := q.QueryRowContext(ctx, `SELECT value FROM rbitr.system_settings WHERE key = $1`, setupBootstrapKey)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func countRows(ctx context.Context, q setupQuerier, query string) (int, error) {
	row := q.QueryRowContext(ctx, query)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func getSetupState(ctx context.Context, q setupQuerier) (setupStateRecord, error) {
	row := q.QueryRowContext(ctx, `SELECT state, COALESCE(last_error, '') FROM rbitr.setup_state WHERE singleton = TRUE`)
	var record setupStateRecord
	if err := row.Scan(&record.State, &record.LastErr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return setupStateRecord{State: setupStateNotStarted}, nil
		}
		return setupStateRecord{}, err
	}
	return record, nil
}

func trySetupInitializeLock(ctx context.Context, q setupQuerier) (bool, error) {
	row := q.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, setupInitializeLockKey)
	var ok bool
	if err := row.Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func findIdempotencyReplay(ctx context.Context, q setupQuerier, key string) (idempotencyReplay, bool, error) {
	row := q.QueryRowContext(
		ctx,
		`SELECT payload_hash, response_json
		FROM rbitr.setup_initialize_idempotency
		WHERE idempotency_key = $1`,
		key,
	)
	var replay idempotencyReplay
	if err := row.Scan(&replay.PayloadHash, &replay.ResponseRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return idempotencyReplay{}, false, nil
		}
		return idempotencyReplay{}, false, err
	}
	return replay, true, nil
}

func checkInitializeReplay(
	ctx context.Context,
	q setupQuerier,
	idempotencyKey string,
	payloadHash string,
) (InitializeResponse, bool, error) {
	if idempotencyKey == "" {
		return InitializeResponse{}, false, nil
	}
	replay, found, err := findIdempotencyReplay(ctx, q, idempotencyKey)
	if err != nil {
		return InitializeResponse{}, false, err
	}
	if !found {
		return InitializeResponse{}, false, nil
	}
	if replay.PayloadHash != payloadHash {
		return InitializeResponse{}, false, ErrIdempotencyConflict
	}
	resp, err := decodeReplayResponse(replay.ResponseRaw)
	if err != nil {
		return InitializeResponse{}, false, err
	}
	return resp, true, nil
}

func storeIdempotencyReplay(
	ctx context.Context,
	q setupExecQuerier,
	key, payloadHash string,
	responseRaw []byte,
	tokenFingerprint, clientIP string,
	now time.Time,
) error {
	_, err := q.ExecContext(
		ctx,
		`INSERT INTO rbitr.setup_initialize_idempotency (
			idempotency_key, payload_hash, response_json, token_fingerprint, client_ip, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		key,
		payloadHash,
		responseRaw,
		nullableString(tokenFingerprint),
		nullableString(clientIP),
		now,
	)
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
		return ErrIdempotencyConflict
	}
	return err
}

func initializePayloadHash(req *InitializeRequest) string {
	if req == nil {
		return ""
	}
	parts := []string{
		strings.TrimSpace(req.TenantName),
		strings.TrimSpace(req.TenantID),
		strings.TrimSpace(req.AdminKey),
		strings.TrimSpace(req.TenantKey),
	}
	return utils.HashString(strings.Join(parts, "\x1f"))
}

func decodeReplayResponse(raw []byte) (InitializeResponse, error) {
	var resp InitializeResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return InitializeResponse{}, err
	}
	return resp, nil
}

func tenantIDExists(ctx context.Context, q setupQuerier, tenantID string) (bool, error) {
	row := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM rbitr.tenants WHERE tenant_id = $1)`, tenantID)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func tenantNameExists(ctx context.Context, q setupQuerier, tenantName string) (bool, error) {
	row := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM rbitr.tenants WHERE LOWER(name) = LOWER($1))`, tenantName)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func generateSecret(prefix string) (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf[:]), nil
}

//nolint:mnd // ignore trimming of keyPrefix.
func keyPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= 14 {
		return trimmed
	}
	return trimmed[:14]
}

func normalizeWriteError(err error, field string) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
		return validationError(field, field+" already exists")
	}
	return err
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
