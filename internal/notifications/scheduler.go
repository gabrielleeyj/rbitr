package notifications

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/models"
	"github.com/gabrielleeyj/rbitr/internal/store"
	"github.com/gabrielleeyj/rbitr/internal/utils"
)

const (
	EventApprovalExpiring    = "APPROVAL.EXPIRING"
	EventApprovalExpired     = "APPROVAL.EXPIRED"
	EventTokenAbuse          = "SECURITY.TOKEN_ABUSE"
	EventPolicyInvalidOutput = "POLICY.INVALID_OUTPUT"
	EventPolicyEvalError     = "POLICY.EVAL_ERROR"
	SeverityWarn             = "WARN"
	SeverityCritical         = "CRITICAL"
	defaultWindow            = 5 * time.Minute
)

// TicketingHook is called when an approval expires to update linked tickets.
type TicketingHook interface {
	OnApprovalExpired(ctx context.Context, tenantID, approvalID string)
}

type ApprovalExpiryScheduler struct {
	Store     store.StoreAPI
	Sender    NotificationSender
	Ticketing TicketingHook
	Interval  time.Duration
	Window    time.Duration
	LockKey   int64
}

func NewApprovalExpiryScheduler(st store.StoreAPI, service *Service, interval, window time.Duration) *ApprovalExpiryScheduler {
	return &ApprovalExpiryScheduler{
		Store:    st,
		Sender:   service,
		Interval: interval,
		Window:   window,
		LockKey:  hashLockKey("approval-expiry-scheduler"),
	}
}

func (s *ApprovalExpiryScheduler) Start(ctx context.Context) {
	if s.Interval == 0 {
		s.Interval = time.Minute
	}
	if s.Window == 0 {
		s.Window = defaultWindow
	}

	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()

	s.run(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.run(ctx)
		}
	}
}

func (s *ApprovalExpiryScheduler) run(ctx context.Context) {
	locked, err := s.Store.TryAdvisoryLock(ctx, s.LockKey)
	if err != nil {
		log.Printf("approval expiry lock failed: %v", err)
		return
	}
	if !locked {
		return
	}
	defer func() {
		if releaseErr := s.Store.ReleaseAdvisoryLock(ctx, s.LockKey); releaseErr != nil {
			log.Printf("approval expiry unlock failed: %v", releaseErr)
		}
	}()

	now := time.Now().UTC()
	expiring, err := s.Store.ListApprovalsExpiring(ctx, now, s.Window)
	if err != nil {
		log.Printf("approval expiry list expiring failed: %v", err)
		return
	}
	for i := range expiring {
		approval := &expiring[i]
		if notifyErr := s.notifyApproval(ctx, approval, EventApprovalExpiring, now); notifyErr != nil {
			log.Printf("approval expiring notify failed: %v", notifyErr)
		}
	}

	expired, err := s.Store.ListApprovalsExpired(ctx, now)
	if err != nil {
		log.Printf("approval expiry list expired failed: %v", err)
		return
	}
	for i := range expired {
		approval := &expired[i]
		if err := s.Store.MarkApprovalExpired(ctx, approval.TenantID, approval.ApprovalRequestID, now); err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrInvalidState) {
				continue
			}
			log.Printf("approval expiry mark expired failed: %v", err)
			continue
		}
		if err := s.notifyApproval(ctx, approval, EventApprovalExpired, now); err != nil {
			log.Printf("approval expired notify failed: %v", err)
		}
		if s.Ticketing != nil {
			go s.Ticketing.OnApprovalExpired(context.Background(), approval.TenantID, approval.ApprovalRequestID) //nolint:gosec // fire-and-forget; must outlive scheduler tick context
		}
	}
}

func (s *ApprovalExpiryScheduler) notifyApproval(ctx context.Context, approval *models.ApprovalRequest, eventType string, now time.Time) error {
	if s.Sender == nil {
		return nil
	}
	message := ApprovalNotificationMessage(approval, eventType, now)
	return s.Sender.Send(ctx, approval.TenantID, NotificationEvent{
		TenantID:   approval.TenantID,
		EventType:  eventType,
		Severity:   SeverityWarn,
		ResourceID: approval.ApprovalRequestID,
	}, message)
}

func ApprovalNotificationMessage(approval *models.ApprovalRequest, eventType string, now time.Time) NotificationMessage {
	expiresIn := approval.ExpiresAt.Sub(now)
	fields := map[string]string{
		"Tenant":   approval.TenantID,
		"Approval": approval.ApprovalRequestID,
		"Action":   approval.ActionType,
		"Risk":     approval.Risk,
	}
	if eventType == EventApprovalExpiring {
		fields["ExpiresIn"] = formatDurationMinutes(expiresIn)
	}
	fields["summary"] = approval.ActionSummary
	return BuildMessage(eventType, fields)
}

func formatDurationMinutes(value time.Duration) string {
	minutes := max(int(value.Minutes()), 0)
	if minutes == 0 {
		return "now"
	}
	return fmt.Sprintf("%dm", minutes)
}

func hashLockKey(value string) int64 {
	sum := utils.HashString(value)
	data := []byte(sum)
	var buf [8]byte
	copy(buf[:], data)
	return int64(binary.LittleEndian.Uint32(buf[:4]))
}
