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
RUN go build -o /out/recalldock ./cmd/recalldock

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates \
    && mv /usr/bin/git /usr/bin/git.real \
    && printf '%s\n' '#!/bin/sh' 'exec /usr/bin/git.real -c credential.helper="store --file=/run/secrets/github_credentials" -c safe.directory=/memory -c user.name="RecallDock" -c user.email="recalldock@local" "$@"' > /usr/local/bin/git \
    && chmod +x /usr/local/bin/git \
    && ln -s /usr/local/bin/git /usr/bin/git
WORKDIR /app
COPY --from=go-builder /out/recalldock /usr/local/bin/recalldock
ENV RECALLDOCK_HOST=0.0.0.0 \
    RECALLDOCK_PORT=18777 \
    RECALLDOCK_STORE_DIR=/memory \
    MEMORYDOCK_HOST=0.0.0.0 \
    MEMORYDOCK_PORT=18777 \
    MEMORYDOCK_STORE_DIR=/memory
EXPOSE 18777
VOLUME ["/memory"]
ENTRYPOINT ["recalldock"]
