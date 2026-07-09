.PHONY: fmt test vet contracts web-build build build-nexusdock run run-nexusdock clean

fmt:
	gofmt -w ./cmd ./internal ./tests

test:
	go test ./...

vet:
	go vet ./...

contracts:
	python3 scripts/check-contracts.py

web-build:
	cd web && npm run build

build: web-build test vet contracts build-nexusdock

build-nexusdock:
	mkdir -p bin
	go build -o bin/nexusdock ./cmd/nexusdock

run: run-nexusdock

run-nexusdock:
	go run ./cmd/nexusdock

clean:
	rm -rf bin web/dist
