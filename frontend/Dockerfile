FROM node:22-alpine AS builder

WORKDIR /app
COPY package*.json ./
RUN npm ci --cache /tmp/npm-cache
COPY . .
RUN npm run build

# ---- Runtime: serve static via nginx ----
FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html

# Nginx config: proxy /api and /ws to backend
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
