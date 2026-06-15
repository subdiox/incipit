.PHONY: all build frontend test test-go vet run docker clean tidy

BINARY := incipit

all: frontend build

## Build the Go binary (expects web/dist to exist; run `make frontend` first).
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/incipit

## Build the frontend SPA into web/dist.
frontend:
	cd frontend && npm install && npm run build

## Run the full headless test suite.
test: test-go

test-go:
	go test ./...

vet:
	go vet ./...

## Run locally against ./library and ./config.
run: build
	INCIPIT_LIBRARY=./library INCIPIT_CONFIG=./config ./$(BINARY)

## Build the single-container Docker image.
docker:
	docker build -t incipit:latest .

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
	rm -rf web/dist/assets
