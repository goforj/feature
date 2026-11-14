.PHONY: test test-unit record-unit test-race test-integration record-integration test-coverage vet tidy modules-check generate check-generated test-tools

test:
	go run ./tools/maintain test-all

test-unit:
	go run ./tools/maintain test-unit

record-unit:
	go run ./tools/maintain record-unit

test-integration:
	go run ./tools/maintain test-integration

record-integration:
	go run ./tools/maintain record-integration

test-coverage: test-unit test-integration

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

.PHONY: release-plan verify-release
release-plan:
	bash scripts/release-plan.sh "$(VERSION)"

verify-release:
	go run ./tools/consumercheck -version "$(VERSION)"

.PHONY: smoke-app
smoke-app:
	go run ./tools/appsmoke -version "$(VERSION)" $(SMOKE_ARGS)
