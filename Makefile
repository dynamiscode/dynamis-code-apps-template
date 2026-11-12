GOVULNCHECK_VERSION ?= v1.7.0
ACTIONLINT_VERSION ?= v1.7.12
GITLEAKS_VERSION ?= v8.30.1

.PHONY: setup setup-hooks fmt-check lint test vet race deps-check secret-check vuln-check workflow-check fuzz-smoke generate-check build image docker-smoke accessibility-smoke webmcp-smoke template-smoke verify js-check test-unit test-postgres logger-injection commit-msg-hook

setup: setup-hooks
	test -f .env || cp .env.example .env
	go mod download
	npm ci

setup-hooks:
	if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then git config core.hooksPath .githooks && test "$$(git config --get core.hooksPath)" = .githooks && git config --local rerere.enabled true && test "$$(git config --get rerere.enabled)" = true; fi

fmt-check:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

js-check:
	node --check internal/web/assets/app.js

doc-links:
	./scripts/check-doc-links.sh

web-conformance:
	./scripts/check-web-conformance.sh

test-unit:
	node --test internal/web/assets/*.test.mjs

test-postgres:
	set -e; \
	name="dynamis-code-test-pg-$$$$"; \
	trap 'docker rm -f "$$name" >/dev/null 2>&1 || true' EXIT; \
	docker run -d --name "$$name" -e POSTGRES_DB=dynamis_code_test -e POSTGRES_USER=postgres -e POSTGRES_HOST_AUTH_METHOD=trust -p 127.0.0.1::5432 postgres:14.24-alpine3.24@sha256:727876d274666da0b92a445390ba093c84b8e9f8343e1c53cd4e9a7ab2d85310 >/dev/null; \
	port="$$(docker port "$$name" 5432/tcp | head -n1 | sed 's/.*://')"; \
	for i in $$(seq 1 60); do docker exec "$$name" pg_isready -h 127.0.0.1 -U postgres -d dynamis_code_test >/dev/null 2>&1 && break; sleep 1; done; \
	docker exec "$$name" pg_isready -h 127.0.0.1 -U postgres -d dynamis_code_test >/dev/null; \
	POSTGRES_TEST_URL="postgres://postgres@127.0.0.1:$$port/dynamis_code_test?sslmode=disable" go test -p 1 ./internal/identity/... ./internal/files/... -count=1

lint: fmt-check vet js-check doc-links web-conformance logger-injection commit-msg-hook

logger-injection:
	./scripts/check-logger-injection.sh
	./scripts/test-check-logger-injection.sh

commit-msg-hook:
	./scripts/test-commit-msg-hook.sh

test: test-unit
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

deps-check:
	go mod verify

secret-check:
	go run github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION) dir --redact --no-banner --exit-code 1 .

vuln-check:
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

workflow-check:
	go run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)
	./scripts/test-check-action-pins.sh
	./scripts/check-action-pins.sh

fuzz-smoke:
	go test ./internal/portability -run '^$$' -fuzz FuzzParseImport -fuzztime 2s
	go test ./internal/webhooks -run '^$$' -fuzz FuzzValidateURL -fuzztime 2s
	go test ./internal/identity -run '^$$' -fuzz FuzzValidatePublicHTTPSURL -fuzztime 2s

generate-check:
	before="$$(git diff -- api/contract.gen.go)"; \
	go generate ./api; \
	after="$$(git diff -- api/contract.gen.go)"; \
	test "$$before" = "$$after"

build:
	go build ./cmd/...

image:
	docker build --build-arg VERSION="$${VERSION:-dev}" --build-arg VCS_REF="$${VCS_REF:-unknown}" --build-arg CREATED="$${CREATED:-unknown}" --build-arg SOURCE="$${SOURCE:-local}" -t "dynamis-code-apps-template:$${VERSION:-dev}" .

docker-smoke:
	./scripts/docker-smoke.sh

webmcp-smoke:
	./scripts/webmcp-smoke.sh

accessibility-smoke:
	./scripts/accessibility-smoke.sh

template-smoke:
	work="$$(mktemp -d /tmp/dynamis-code-template-smoke.XXXXXX)"; \
	go run ./cmd/template-init -output "$$work/app" -name "Smoke App" -module example.com/smoke/app -repository https://github.com/smoke/app -security-url https://github.com/smoke/app/security/advisories/new -maintainer @smoke/maintainers -profiles Core,Identity,Agent -source https://example.com/dynamis-code/template -commit 0000000000000000000000000000000000000000; \
	cd "$$work/app" && go test ./...

verify: lint test race deps-check workflow-check generate-check build template-smoke
