.PHONY: fmt test vet contracts web-build build build-nexus build-recalldock run run-nexus run-recalldock clean

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

build: web-build test vet contracts build-nexus

build-nexus:
	mkdir -p bin
	go build -o bin/nexus ./cmd/nexus

build-recalldock:
	mkdir -p bin
	go build -o bin/recalldock ./cmd/recalldock

run: run-nexus

run-nexus:
	go run ./cmd/nexus

run-recalldock:
	go run ./cmd/recalldock

clean:
	rm -rf bin
