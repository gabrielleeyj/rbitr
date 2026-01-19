#!/bin/bash
# vim: ai:ts=8:sw=8:noet
set -eufo pipefail
IFS=$'\t\n'

command -v go >/dev/null 2>&1 || {
	echo 'Please install go or use image that has it'
	exit 1
}

# Install gotestsum for printing formatted test output and a summary of the test run
go run gotest.tools/gotestsum@latest \
	--junitfile junit.xml \
	--format testname -- \
	-race -covermode=atomic \
	-coverpkg=$(go list ./... | grep -vE 'mock|_test' | paste -sd "," -) \
	-coverprofile=test_coverage.txt \
	-tags=unit ./...

# Clean up coverage file by removing mocks and test files
grep -vE "_mock.go|_test.go" test_coverage.txt >filtered.txt && mv filtered.txt test_coverage.txt

# Print total test coverage
go tool cover -func=test_coverage.txt | tail -n1 | awk '{print "Total test coverage: " $3}'

# Convert coverage to Cobertura format for CI integration
go run github.com/boumenot/gocover-cobertura@latest <test_coverage.txt >test_coverage.xml
