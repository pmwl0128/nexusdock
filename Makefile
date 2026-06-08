.PHONY: fmt test vet contracts web-build build build-nexus build-memorydock run run-nexus run-memorydock clean

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

build:
	$(MAKE) build-nexus
	$(MAKE) build-memorydock

build-nexus:
	go build -o bin/nexus-server ./cmd/nexus-server
	go build -o bin/nexus-worker ./cmd/nexus-worker

build-memorydock:
	go build -o bin/memorydock ./cmd/memorydock

run:
	$(MAKE) run-nexus

run-nexus:
	go run ./cmd/nexus-server

run-memorydock:
	go run ./cmd/memorydock

clean:
	rm -rf bin
