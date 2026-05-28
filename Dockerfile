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
RUN go build -o /out/memorydock ./cmd/memorydock

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY --from=go-builder /out/memorydock /usr/local/bin/memorydock
ENV MEMORYDOCK_HOST=0.0.0.0 \
    MEMORYDOCK_PORT=18777 \
    MEMORYDOCK_STORE_DIR=/memory
EXPOSE 18777
VOLUME ["/memory"]
ENTRYPOINT ["memorydock"]
