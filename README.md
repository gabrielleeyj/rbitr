1. Run migrations: `goose -dir migrations postgres "$DATABASE_URL" up`
2. Start mock tool and gateway: go run `./cmd/mocktool` and `go run ./cmd/gateway`
3. Run tests: `go test ./...`
