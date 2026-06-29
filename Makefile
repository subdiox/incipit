.PHONY: all build frontend test test-go vet run seed docker clean tidy

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

## Run locally against ./config. The Calibre library path is chosen during
## first-run setup (or set INCIPIT_LIBRARY=./library to pre-configure it).
run: build
	INCIPIT_CONFIG=./config ./$(BINARY)

## Populate ./library with a few sample CBZ comics (use ARGS="-reset" to replace).
seed:
	INCIPIT_LIBRARY=./library go run ./cmd/seed $(ARGS)

## Build the single-container Docker image.
docker:
	docker build -t incipit:latest .

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
	rm -rf web/dist/assets
