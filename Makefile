.PHONY: fmt-check test vet race generate-check build image docker-smoke accessibility-smoke webmcp-smoke template-smoke verify

fmt-check:
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))"

test:
	go test ./...

vet:
	go vet ./...

race:
	go test -race ./...

generate-check:
	go generate ./api
	git diff --exit-code -- api/contract.gen.go

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
	go run ./cmd/template-init -output "$$work/app" -name "Smoke App" -module example.com/smoke/app -source https://example.com/dynamis-code/template -commit 0000000000000000000000000000000000000000; \
	cd "$$work/app" && go test ./...

verify: fmt-check test vet race generate-check build template-smoke
