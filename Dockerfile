# syntax=docker/dockerfile:1

# --- Stage 1: build the frontend SPA ---
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install
COPY frontend/ ./
# Vite is configured to emit to ../web/dist (i.e. /app/web/dist).
RUN npm run build

# --- Stage 2: build the Go binary (embeds web/dist) ---
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace the placeholder dist with the freshly built frontend.
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /incipit ./cmd/incipit

# --- Stage 3: minimal runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="Incipit" \
      org.opencontainers.image.description="Lightweight, modern server for Calibre comic libraries (CBZ)" \
      org.opencontainers.image.source="https://github.com/incipit/incipit"

COPY --from=backend /incipit /incipit

ENV INCIPIT_ADDR=":8080" \
    INCIPIT_LIBRARY="/library" \
    INCIPIT_CONFIG="/config"

EXPOSE 8080
VOLUME ["/library", "/config"]
USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/incipit", "healthcheck"]

ENTRYPOINT ["/incipit"]
