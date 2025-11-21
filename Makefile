VERSION ?= $(shell runner/otelcol-contrib -v | cut -d ' ' -f 3)
BUILD_DIR = build
CGO_ENABLED ?= 0
GOARCH ?= $(shell go env GOARCH)
GOOS ?= $(shell go env GOOS)
DOCKERHUB_REPO = netboxlabs
COMMIT_HASH = $(shell git rev-parse --short HEAD)
INF_LATEST_VERSION := $(shell curl -L -s -H 'Accept: application/json' https://github.com/netboxlabs/opentelemetry-infinity/releases/latest | sed -E 's/.*"tag_name":"([^"]*)".*/\1/')

getotelcol:
	curl -L -o /tmp/otelcol-contrib-$(GOARCH)$(GOARM).zip https://github.com/netboxlabs/opentelemetry-infinity/releases/download/$(INF_LATEST_VERSION)/otelcol-contrib-$(GOARCH)$(GOARM).zip
	unzip -q /tmp/otelcol-contrib-$(GOARCH)$(GOARM).zip -d /tmp/
	mv /tmp/otelcol-contrib runner/otelcol-contrib
	rm -rf /tmp/otelcol-contrib*

.PHONY: build
build:
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=$(GOARCH) GOARM=$(GOARM) go build -ldflags="-s -w" -trimpath -o ${BUILD_DIR}/otlpinf cmd/main.go

test:
	go test -v ./...

.PHONY: test-coverage
test-coverage:
	@mkdir -p .coverage
	@go test -race -cover -json -coverprofile=.coverage/cover.out.tmp ./... | grep -Ev "cmd" | tparse -format=markdown > .coverage/test-report.md
	@cat .coverage/cover.out.tmp | grep -Ev "cmd" > .coverage/cover.out
	@go tool cover -func=.coverage/cover.out | grep total | awk '{print substr($$3, 1, length($$3)-1)}' > .coverage/coverage.txt

.PHONY: lint
lint:
	@golangci-lint run ./... --config .github/golangci.yaml

.PHONY: fix-lint
fix-lint:
	@golangci-lint run ./... --config .github/golangci.yaml --fix

container:
	docker build --no-cache \
	  --tag=$(DOCKERHUB_REPO)/opentelemetry-infinity:develop \
	  --tag=$(DOCKERHUB_REPO)/opentelemetry-infinity:develop-$(COMMIT_HASH) \
	  -f docker/Dockerfile .

release:
	docker build --no-cache \
	  --tag=$(DOCKERHUB_REPO)/opentelemetry-infinity:latest \
	  --tag=$(DOCKERHUB_REPO)/opentelemetry-infinity:$(VERSION) \
	  --tag=$(DOCKERHUB_REPO)/opentelemetry-infinity:$(VERSION)-$(COMMIT_HASH) \
	  -f docker/Dockerfile .
