.PHONY: help test lint fmt build build-freebsd docker-build docker-up docker-down fake-run fake-netflow migrate clean test-integration test-opnsense-api test-netflow-listener netflow-record netflow-replay netflow-decode package-opnsense

APP := otm

help:
	@echo "OTM development targets"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  help                    Show this help"
	@echo "  test                    Run unit tests"
	@echo "  lint                    Run go vet"
	@echo "  fmt                     Format Go files"
	@echo "  build                   Build local binary"
	@echo "  build-freebsd           Build FreeBSD amd64 binary for OPNsense"
	@echo "  docker-build            Build Docker image"
	@echo "  docker-up               Start Docker Compose stack"
	@echo "  docker-down             Stop Docker Compose stack"
	@echo "  fake-run                Build and run local app with test bind addresses"
	@echo "  fake-netflow            Placeholder for fake NetFlow sender"
	@echo "  migrate                 Validate config/migration command path"
	@echo "  clean                   Remove build outputs"
	@echo "  test-integration        Run opt-in integration tests"
	@echo "  test-opnsense-api       Validate configured OPNsense API"
	@echo "  test-netflow-listener   Print manual NetFlow listener test guidance"
	@echo "  netflow-record          Placeholder for NetFlow capture utility"
	@echo "  netflow-replay          Placeholder for NetFlow replay utility"
	@echo "  netflow-decode          Placeholder for NetFlow decode utility"
	@echo "  package-opnsense        Placeholder for OPNsense plugin packaging"

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
	go run ./cmd/otm version
	@echo "NetFlow fixture sender is planned; use nc/socat or OPNsense export for now."

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
	@echo "Planned: otmctl netflow record"

netflow-replay:
	@echo "Planned: otmctl netflow replay"

netflow-decode:
	@echo "Planned: otmctl netflow decode"

package-opnsense: build-freebsd
	@echo "OPNsense plugin packaging is planned after the standalone service proves the path."
