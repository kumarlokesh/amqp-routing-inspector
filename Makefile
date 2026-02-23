BINARY := amqp-routing-inspector
PKG := ./...

PROJECT_NAME ?= $(or $(COMPOSE_PROJECT_NAME),$(notdir $(CURDIR)))

.PHONY: help deps fmt test test-race coverage build run run-json docker-up docker-down test-integration smoke-docker smoke-docker-deliver docker-clean-oneoff

help:
	@echo "Available targets:"
	@echo "  deps            - download and tidy go dependencies"
	@echo "  fmt             - gofmt all Go files"
	@echo "  test            - run unit tests"
	@echo "  test-race       - run tests with race detector"
	@echo "  coverage        - generate coverage.out"
	@echo "  build           - compile binary into ./bin"
	@echo "  run             - run inspector with sample config"
	@echo "  run-json        - run inspector in JSON mode"
	@echo "  docker-up       - start RabbitMQ and inspector stack"
	@echo "  docker-down     - stop docker stack"
	@echo "  test-integration- run docker-backed integration tests"
	@echo "  smoke-docker    - run docker smoke test for cli/json/dot outputs"
	@echo "  smoke-docker-deliver - run docker smoke test that waits for deliver events (proves destinations)"
	@echo "  docker-clean-oneoff  - remove any one-off inspector containers left behind"

deps:
	go mod tidy

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

test:
	go test $(PKG)

test-race:
	go test -race $(PKG)

coverage:
	go test -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out

build:
	mkdir -p bin
	go build -trimpath -o bin/$(BINARY) ./cmd/amqp-routing-inspector

run:
	go run ./cmd/amqp-routing-inspector --config configs/config.example.yaml

run-json:
	go run ./cmd/amqp-routing-inspector --config configs/config.example.yaml --output json --max-events 10

docker-up:
	docker compose up --build -d
	@echo "Enable firehose tracing: docker compose exec rabbitmq rabbitmqctl trace_on -p /"

docker-down:
	docker compose down

test-integration:
	AMQP_INTEGRATION=1 go test -tags=integration ./test/integration/...

smoke-docker:
	bash scripts/smoke_docker.sh

smoke-docker-deliver:
	bash scripts/smoke_docker_deliver.sh

docker-clean-oneoff:
	@docker rm -f $$(docker ps -aq \
		--filter "label=com.docker.compose.project=$(PROJECT_NAME)" \
		--filter "label=com.docker.compose.service=inspector" \
		--filter "label=com.docker.compose.oneoff=True" 2>/dev/null) >/dev/null 2>&1 || true
