.PHONY: test test-unit record-unit test-race test-coverage vet tidy modules-check generate check-generated test-tools

test:
	go test ./...

test-unit:
	go run ./tools/maintain test-unit

record-unit:
	go run ./tools/maintain record-unit

test-coverage: test-unit

test-race:
	go test -race ./...

vet:
	go vet ./...

tidy:
	go mod tidy

modules-check:
	@test "$$(find . -name go.mod -not -path './.git/*' | sort)" = './go.mod'

generate:
	go run ./tools/maintain generate

check-generated:
	go run ./tools/maintain check-generated

test-tools:
	go test ./internal/devtools ./tools/maintain
