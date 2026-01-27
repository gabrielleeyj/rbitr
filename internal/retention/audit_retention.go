package retention

import (
	"context"
	"encoding/binary"
	"log"
	"time"

	"github.com/gabrielleeyj/rbitr/internal/store"
)

type AuditRetentionScheduler struct {
	Store    store.StoreAPI
	Interval time.Duration
	LockKey  int64
}

func NewAuditRetentionScheduler(st store.StoreAPI, interval time.Duration) *AuditRetentionScheduler {
	return &AuditRetentionScheduler{
		Store:    st,
		Interval: interval,
		LockKey:  hashLockKey("audit-retention-scheduler"),
	}
}

func (s *AuditRetentionScheduler) Start(ctx context.Context) {
	if s.Interval == 0 {
		s.Interval = 24 * time.Hour
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

func (s *AuditRetentionScheduler) run(ctx context.Context) {
	locked, err := s.Store.TryAdvisoryLock(ctx, s.LockKey)
	if err != nil {
		log.Printf("audit retention lock failed: %v", err)
		return
	}
	if !locked {
		return
	}
	defer func() {
		if err := s.Store.ReleaseAdvisoryLock(ctx, s.LockKey); err != nil {
			log.Printf("audit retention unlock failed: %v", err)
		}
	}()

	days, err := s.Store.GetAuditRetentionDays(ctx)
	if err != nil || days <= 0 {
		days = 365
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	if _, err := s.Store.DeleteAuditEventsBefore(ctx, cutoff); err != nil {
		log.Printf("audit retention cleanup failed: %v", err)
	}
}

func hashLockKey(name string) int64 {
	hash := []byte(name)
	if len(hash) < 8 {
		padded := make([]byte, 8)
		copy(padded, hash)
		hash = padded
	}
	return int64(binary.BigEndian.Uint64(hash[:8]))
}
