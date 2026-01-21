package db

import (
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestConnect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		openFunc  func(string, string) (sqlmock.Sqlmock, *sql.DB, error)
		expectErr bool
	}{
		{
			name: "open error",
			openFunc: func(_, _ string) (sqlmock.Sqlmock, *sql.DB, error) {
				return nil, nil, errors.New("open failed")
			},
			expectErr: true,
		},
		{
			name: "ping error",
			openFunc: func(_, _ string) (sqlmock.Sqlmock, *sql.DB, error) {
				db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
				if err != nil {
					return nil, nil, err
				}
				mock.ExpectPing().WillReturnError(errors.New("ping failed"))
				return mock, db, nil
			},
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

			conn, err := connect(open, "dsn")
			if conn != nil {
				_ = conn.Close()
			}

			if tc.expectErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mock != nil {
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatalf("expectations: %v", err)
				}
			}
		})
	}
}
