GOVULNCHECK_VERSION ?= v1.7.0
ACTIONLINT_VERSION ?= v1.7.12
GITLEAKS_VERSION ?= v8.30.1

.PHONY: setup fmt-check lint test vet race deps-check secret-check vuln-check workflow-check generate-check build image docker-smoke accessibility-smoke webmcp-smoke template-smoke verify

setup:
	test -f .env || cp .env.example .env
	go mod download
	npm ci

fmt-check:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

lint: fmt-check vet

test:
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
	./scripts/check-action-pins.sh

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
