SHELL := /bin/sh

MOCKERY := mockery

.PHONY: mocks
mocks:
	GOCACHE=$$(mktemp -d) $(MOCKERY) --config mockery.yaml

.PHONY: demo
demo:
	./scripts/demo.sh
