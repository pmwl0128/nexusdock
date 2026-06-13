.PHONY: fmt test vet contracts web-build build build-memorydock run run-memorydock clean

fmt:
	gofmt -w .

test:
	go test ./...

vet:
	go vet ./...

contracts:
	python3 scripts/check-contracts.py

web-build:
	cd web && npm run build

build: web-build test vet contracts build-memorydock

build-memorydock:
	mkdir -p bin
	go build -o bin/memorydock ./cmd/memorydock

run: run-memorydock

run-memorydock:
	go run ./cmd/memorydock

clean:
	rm -rf bin
