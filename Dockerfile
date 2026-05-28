FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /out/memorydock ./cmd/memorydock

FROM alpine:3.20

RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY --from=builder /out/memorydock /usr/local/bin/memorydock

ENV MEMORYDOCK_HOST=0.0.0.0 \
    MEMORYDOCK_PORT=18777 \
    MEMORYDOCK_STORE_DIR=/memory

EXPOSE 18777
VOLUME ["/memory"]
ENTRYPOINT ["memorydock"]

