.PHONY: test test-unit record-unit test-race test-coverage vet tidy modules-check generate check-generated test-tools

test:
	go run ./tools/maintain test-all

test-unit:
	go run ./tools/maintain test-unit

record-unit:
	go run ./tools/maintain record-unit

test-coverage: test-unit

test-race:
	go run ./tools/maintain test-race

vet:
	go run ./tools/maintain vet

tidy:
	go run ./tools/maintain tidy

modules-check:
	go run ./tools/maintain modules-check

generate:
	go run ./tools/maintain generate

check-generated:
	go run ./tools/maintain check-generated

test-tools:
	go test ./internal/devtools ./tools/maintain
