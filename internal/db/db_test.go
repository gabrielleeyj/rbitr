package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestConnect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		openFunc  func(string, string) (sqlmock.Sqlmock, *sql.DB, error)
		pool      PoolConfig
		maxOpen   int
		expectErr bool
	}{
		{
			name: "open error",
			openFunc: func(_, _ string) (sqlmock.Sqlmock, *sql.DB, error) {
				return nil, nil, errors.New("open failed")
			},
			pool:      PoolConfig{},
			expectErr: true,
		},
		{
			name: "ping error",
			openFunc: func(_, _ string) (sqlmock.Sqlmock, *sql.DB, error) {
				db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
				if err != nil {
					return nil, nil, err
				}
				_ = db.Close()
				return mock, db, nil
			},
			pool:      PoolConfig{},
			expectErr: true,
		},
		{
			name: "success",
			openFunc: func(_, _ string) (sqlmock.Sqlmock, *sql.DB, error) {
				db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
				if err != nil {
					return nil, nil, err
				}
				mock.ExpectPing()
				return mock, db, nil
			},
			pool: PoolConfig{
				MaxOpenConns:    42,
				MaxIdleConns:    7,
				ConnMaxLifetime: 10 * time.Minute,
				ConnMaxIdleTime: 2 * time.Minute,
			},
			maxOpen: 42,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				mock sqlmock.Sqlmock
				db   *sql.DB
				err  error
			)

			open := func(driver, dsn string) (*sql.DB, error) {
				mock, db, err = tc.openFunc(driver, dsn)
				if err != nil {
					return nil, err
				}
				return db, nil
			}

			conn, err := connect(open, "dsn", tc.pool)

			if tc.expectErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.expectErr && conn != nil && tc.maxOpen > 0 && conn.Stats().MaxOpenConnections != tc.maxOpen {
				t.Fatalf("expected max open connections %d got %d", tc.maxOpen, conn.Stats().MaxOpenConnections)
			}
			if conn != nil {
				_ = conn.Close()
			}
			if mock != nil {
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatalf("expectations: %v", err)
				}
			}
		})
	}
}

func TestNormalizePoolConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    PoolConfig
		expected PoolConfig
	}{
		{
			name:  "defaults applied",
			input: PoolConfig{},
			expected: PoolConfig{
				MaxOpenConns:    defaultMaxOpenConns,
				MaxIdleConns:    defaultMaxIdleConns,
				ConnMaxLifetime: defaultConnMaxLifetime,
				ConnMaxIdleTime: defaultConnMaxIdleTime,
			},
		},
		{
			name: "invalid values corrected",
			input: PoolConfig{
				MaxOpenConns:    -1,
				MaxIdleConns:    -1,
				ConnMaxLifetime: -1,
				ConnMaxIdleTime: 0,
			},
			expected: PoolConfig{
				MaxOpenConns:    defaultMaxOpenConns,
				MaxIdleConns:    defaultMaxIdleConns,
				ConnMaxLifetime: defaultConnMaxLifetime,
				ConnMaxIdleTime: defaultConnMaxIdleTime,
			},
		},
		{
			name: "idle capped by open",
			input: PoolConfig{
				MaxOpenConns:    5,
				MaxIdleConns:    10,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Second,
			},
			expected: PoolConfig{
				MaxOpenConns:    5,
				MaxIdleConns:    5,
				ConnMaxLifetime: time.Minute,
				ConnMaxIdleTime: time.Second,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePoolConfig(tc.input)
			if got != tc.expected {
				t.Fatalf("expected %#v got %#v", tc.expected, got)
			}
		})
	}
}
