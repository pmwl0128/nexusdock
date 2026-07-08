FROM node:26-alpine AS web-builder
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web ./
COPY internal/httpx/web_dist ../internal/httpx/web_dist
RUN npm run build

FROM golang:1.26-alpine AS go-builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
COPY --from=web-builder /src/internal/httpx/web_dist ./internal/httpx/web_dist
RUN go build -o /out/nexus ./cmd/nexus

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates \
    && mv /usr/bin/git /usr/bin/git.real \
    && printf '%s\n' '#!/bin/sh' 'exec /usr/bin/git.real -c credential.helper="store --file=/run/secrets/github_credentials" -c safe.directory=/recall -c user.name="Recall" -c user.email="recall@local" "$@"' > /usr/local/bin/git \
    && chmod +x /usr/local/bin/git \
    && ln -s /usr/local/bin/git /usr/bin/git
WORKDIR /app
COPY --from=go-builder /out/nexus /usr/local/bin/nexus
ENV NEXUS_HOST=0.0.0.0 \
    NEXUS_PORT=18777 \
    NEXUS_DATA_DIR=/var/lib/nexus \
    RECALL_REPO_DIR=/recall
EXPOSE 18777
VOLUME ["/var/lib/nexus", "/recall"]
ENTRYPOINT ["nexus"]
