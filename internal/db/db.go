package db

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Connect(connStr string) (*sql.DB, error) {
	return connect(sql.Open, connStr)
}

func connect(open func(string, string) (*sql.DB, error), connStr string) (*sql.DB, error) {
	const pingTimeout = 5 * time.Second

	db, err := open("pgx", connStr)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}
