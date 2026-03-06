SHELL := /bin/sh

MOCKERY := mockery

.PHONY: mocks
mocks:
	GOCACHE=$$(mktemp -d) $(MOCKERY) --config mockery.yml

.PHONY: demo
demo:
	./scripts/demo.sh

.PHONY: dev-up
dev-up:
	docker compose -f docker-compose.dev.yml up --build

.PHONY: dev-down
dev-down:
	docker compose -f docker-compose.dev.yml down

.PHONY: dev-bootstrap
dev-bootstrap:
	./scripts/dev/bootstrap.sh
