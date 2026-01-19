SHELL := /bin/sh

MOCKERY := mockery

.PHONY: mocks
mocks:
	$(MOCKERY)
