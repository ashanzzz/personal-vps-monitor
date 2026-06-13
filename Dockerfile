# ============================================================
# Stage 1: Build frontend
# ============================================================
FROM node:22-alpine AS frontend-builder

WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ============================================================
# Stage 2: Backend dependencies
# ============================================================
FROM python:3.12-slim AS backend-builder

WORKDIR /app
COPY backend/requirements.txt ./
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# ============================================================
# Stage 3: Final image — nginx + supervisord + uvicorn
# ============================================================
FROM python:3.12-slim

# Install nginx + supervisor
RUN apt-get update && apt-get install -y --no-install-recommends \
    nginx \
    supervisor \
    && rm -rf /var/lib/apt/lists/*

# Copy Python packages
COPY --from=backend-builder /install /usr/local

# Copy backend source
WORKDIR /app
COPY backend/ ./backend/

# Copy frontend build
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html

# nginx config
COPY deploy/nginx.conf /etc/nginx/sites-available/default

# supervisord config
COPY deploy/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Data volume for SQLite
RUN mkdir -p /app/data
VOLUME ["/app/data"]

EXPOSE 80

CMD ["supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
