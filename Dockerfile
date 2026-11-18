# ============================================================
# Stage 1: Build Go static binary
# ============================================================
FROM golang:alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /src/bin/vps-hub ./cmd/vps-hub

# ============================================================
# Stage 2: Minimal runtime image (< 20MB)
# ============================================================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /src/bin/vps-hub /usr/local/bin/vps-hub

EXPOSE 3338 3337
VOLUME ["/app/data"]

ENTRYPOINT ["/usr/local/bin/vps-hub"]
CMD ["daemon"]
