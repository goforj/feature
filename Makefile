.PHONY: test test-unit test-race test-coverage vet tidy modules-check generate check-generated test-tools

test:
	go test ./...

test-unit:
	python3 scripts/test-unit.py

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
	python3 tools/generate.py

check-generated:
	python3 scripts/check-generated.py

test-tools:
	python3 -m unittest discover -s tools -p 'test_*.py'
