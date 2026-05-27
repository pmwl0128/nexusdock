.PHONY: fmt test build run clean

fmt:
	gofmt -w .

test:
	go test ./...

build:
	go build -o memorydock ./cmd/memorydock

run:
	go run ./cmd/memorydock

clean:
	rm -f memorydock

