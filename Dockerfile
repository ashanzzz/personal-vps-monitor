# ============================================================
# Stage 1: Build React Frontend (Vite + Ant Design)
# ============================================================
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci --prefer-offline || npm install
COPY frontend/ ./
RUN npm run build

# ============================================================
# Stage 2: Build Go static binary (vps-hub CLI & MCP server)
# ============================================================
FROM golang:alpine AS go-builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /src/bin/vps-hub ./cmd/vps-hub

# ============================================================
# Stage 3: Python dependencies builder
# ============================================================
FROM python:3.12-slim AS backend-builder
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends gcc && rm -rf /var/lib/apt/lists/*
COPY backend/requirements.txt ./
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# ============================================================
# Stage 4: Production Runtime Image (Full-Stack Monolith)
# ============================================================
FROM python:3.12-slim

# Install system dependencies: nginx, supervisor, curl, tzdata
RUN apt-get update && apt-get install -y --no-install-recommends \
    nginx \
    supervisor \
    curl \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Copy Python packages from builder
COPY --from=backend-builder /install /usr/local

# Copy Go binary
COPY --from=go-builder /src/bin/vps-hub /usr/local/bin/vps-hub

# Copy Frontend static dist
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html

# Copy Backend application
WORKDIR /app
COPY backend/ /app/backend/

# Nginx & Supervisord configurations
COPY deploy/nginx.conf /etc/nginx/sites-available/default
COPY deploy/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Persistent data directory
RUN mkdir -p /app/data && chmod 777 /app/data
VOLUME ["/app/data"]

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD curl -f http://127.0.0.1/api/version || curl -f http://127.0.0.1:3338/health || exit 1

CMD ["supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
