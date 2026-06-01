.PHONY: help test lint fmt build build-freebsd docker-build docker-up docker-down fake-run fake-netflow migrate clean test-integration test-opnsense-api test-netflow-listener netflow-record netflow-replay netflow-decode package-opnsense

APP := otm
NETFLOW_LISTEN ?= 0.0.0.0:2055
NETFLOW_ALLOW ?=
NETFLOW_DURATION ?= 5m
NETFLOW_OUT ?= captures/netflow.otmcap
NETFLOW_IN ?= $(NETFLOW_OUT)
NETFLOW_TARGET ?= 127.0.0.1:2055
NETFLOW_SPEED ?= 10

help:
	@echo "OTM development targets"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Core:"
	@echo "  help                    Show this help"
	@echo "  test                    Run unit tests"
	@echo "  lint                    Run go vet"
	@echo "  fmt                     Format Go files"
	@echo "  build                   Build local binary"
	@echo "  build-freebsd           Build FreeBSD amd64 binary for OPNsense"
	@echo "  clean                   Remove build outputs"
	@echo ""
	@echo "Runtime:"
	@echo "  fake-run                Build and run local app with test bind addresses"
	@echo "  migrate                 Validate config/migration command path"
	@echo "  test-opnsense-api       Validate configured OPNsense API"
	@echo "  test-netflow-listener   Print manual NetFlow listener test guidance"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build            Build Docker image"
	@echo "  docker-up               Start Docker Compose stack"
	@echo "  docker-down             Stop Docker Compose stack"
	@echo ""
	@echo "NetFlow utilities:"
	@echo "  fake-netflow            Print NetFlow utility guidance"
	@echo "  netflow-record          Record UDP packets to NETFLOW_OUT"
	@echo "  netflow-replay          Replay NETFLOW_IN to NETFLOW_TARGET"
	@echo "  netflow-decode          Decode/summarize NETFLOW_IN"
	@echo ""
	@echo "Integration and packaging:"
	@echo "  test-integration        Run opt-in integration tests"
	@echo "  package-opnsense        Placeholder for OPNsense plugin packaging"
	@echo ""
	@echo "NetFlow variables:"
	@echo "  NETFLOW_LISTEN=$(NETFLOW_LISTEN)"
	@echo "  NETFLOW_ALLOW=$(NETFLOW_ALLOW)"
	@echo "  NETFLOW_DURATION=$(NETFLOW_DURATION)"
	@echo "  NETFLOW_OUT=$(NETFLOW_OUT)"
	@echo "  NETFLOW_IN=$(NETFLOW_IN)"
	@echo "  NETFLOW_TARGET=$(NETFLOW_TARGET)"
	@echo "  NETFLOW_SPEED=$(NETFLOW_SPEED)"

test:
	go test ./...

lint:
	go vet ./...

fmt:
	gofmt -w $$(fd '\.go$$' .)

build:
	mkdir -p bin
	go build -o bin/$(APP) ./cmd/otm

build-freebsd:
	mkdir -p bin
	GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 go build -o bin/$(APP)-freebsd-amd64 ./cmd/otm

docker-build:
	docker build -t otm:dev .

docker-up:
	docker compose up --build

docker-down:
	docker compose down

fake-run: build
	OTM_WEB_ADDR=127.0.0.1:8080 OTM_NETFLOW_ADDR=127.0.0.1:2055 ./bin/$(APP)

fake-netflow:
	@echo "Use netflow-record to capture from OPNsense, netflow-decode to inspect, and netflow-replay to replay."

migrate:
	go run ./cmd/otm validate-config

clean:
	rm -rf bin

test-integration:
	go test ./... -run Integration

test-opnsense-api:
	go run ./cmd/otm validate-opnsense

test-netflow-listener:
	@echo "Start OTM and verify /api/netflow/status after OPNsense exports packets."

netflow-record:
	mkdir -p $$(dirname "$(NETFLOW_OUT)")
	go run ./cmd/otm netflow record --listen "$(NETFLOW_LISTEN)" --allow-exporter "$(NETFLOW_ALLOW)" --duration "$(NETFLOW_DURATION)" --out "$(NETFLOW_OUT)"

netflow-replay:
	go run ./cmd/otm netflow replay --in "$(NETFLOW_IN)" --target "$(NETFLOW_TARGET)" --speed "$(NETFLOW_SPEED)"

netflow-decode:
	go run ./cmd/otm netflow decode --in "$(NETFLOW_IN)"

package-opnsense: build-freebsd
	@echo "OPNsense plugin packaging is planned after the standalone service proves the path."
