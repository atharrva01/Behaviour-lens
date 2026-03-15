# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy dependency files first — allows Docker to cache go mod download layer.
COPY go.mod ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# Build the server binary.
# CGO_ENABLED=0  → fully static binary (no libc dependency).
# -trimpath      → remove local file system paths from the binary.
# -ldflags ...   → strip debug info to reduce binary size.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /server ./cmd/server

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

# Non-root user for security.
RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /home/app

COPY --from=builder /server .

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["./server"]
