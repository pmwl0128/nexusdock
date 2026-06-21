.PHONY: fmt test vet contracts web-build build build-recalldock build-memorydock run run-recalldock run-memorydock clean

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

build: web-build test vet contracts build-recalldock

build-recalldock:
	mkdir -p bin
	go build -o bin/recalldock ./cmd/recalldock

build-memorydock: build-recalldock

run: run-recalldock

run-recalldock:
	go run ./cmd/recalldock

run-memorydock: run-recalldock

clean:
	rm -rf bin
