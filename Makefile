.PHONY: coverage/check run build test test/unit test/integration test/coverage lint swagger mocks up down logs system/up system/down system/logs

COVERAGE_MINIMUM = 80
COVERAGE_PACKAGES = $(shell go list ./internal/... | grep -v -e /mocks -e /integration | paste -sd, -)

run:
	go run ./cmd/api/main.go

build:
	go build -o bin/api ./cmd/api/main.go

test:
	go test ./...

test/unit:
	go test $$(go list ./... | grep -v /internal/integration)

test/integration:
	go test ./internal/integration/... -timeout 300s

test/coverage:
	go test ./... -coverpkg=$(COVERAGE_PACKAGES) -coverprofile=coverage.out -covermode=atomic -timeout 600s
	go tool cover -func=coverage.out | tail -1

coverage/check:
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {sub("%","",$$3); print $$3}'); \
	echo "total coverage: $$total% (minimum $(COVERAGE_MINIMUM)%)"; \
	awk -v total="$$total" -v minimum="$(COVERAGE_MINIMUM)" 'BEGIN { exit (total+0 < minimum+0) ? 1 : 0 }'

mocks:
	~/go/bin/darwin_amd64/mockery

swagger:
	~/go/bin/swag init -g cmd/api/main.go

scan:
	trivy fs --scanners vuln,secret,misconfig .

up:
	docker-compose up --build -d

down:
	docker-compose down

logs:
	docker-compose logs -f api

system/up:
	docker-compose -f docker-compose.system.yml up --build -d

system/down:
	docker-compose -f docker-compose.system.yml down -v

system/logs:
	docker-compose -f docker-compose.system.yml logs -f os-service billing-service execution-service
