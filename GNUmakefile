default: fmt lint install generate

OAPI_CODEGEN_VERSION ?= v2.9.0
OAPI_RUNTIME_VERSION ?= v1.9.1

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

generate: check-oapi-version
	go generate ./internal/client/...
	cd tools; go generate ./...

# The pinned codegen and runtime versions must agree between this Makefile,
# the go:generate line and go.mod.
check-oapi-version:
	@grep -q "oapi-codegen@$(OAPI_CODEGEN_VERSION) " internal/client/doc.go || { echo "doc.go go:generate line does not use oapi-codegen $(OAPI_CODEGEN_VERSION)"; exit 1; }
	@got=$$(go list -m -f '{{.Version}}' github.com/oapi-codegen/runtime); \
	if [ "$$got" != "$(OAPI_RUNTIME_VERSION)" ]; then echo "oapi-codegen runtime: go.mod $$got, Makefile $(OAPI_RUNTIME_VERSION)"; exit 1; fi

fmt:
	gofmt -s -w -e .

test:
	go test -v -cover -timeout=120s -parallel=10 ./...

# Acceptance tests need a Ramble server and a token: run testacc-env-up, then
# '. ./.testacc.env && make testacc', then testacc-env-down.
testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

testacc-env-up:
	scripts/testacc-env.sh up

testacc-env-down:
	scripts/testacc-env.sh down

.PHONY: fmt lint test testacc testacc-env-up testacc-env-down build install generate check-oapi-version
