GO_SOURCES := $(shell find cmd generated internal migrations tests -type f -name '*.go' -print | sort)
WEB_INSTALL_STAMP := web/node_modules/.install-stamp

.PHONY: fmt fmt-check test test-race vet tidy-check contracts repository-check web-deps web-build build build-nexusdock check ci run run-nexusdock clean

fmt:
	gofmt -w $(GO_SOURCES)

fmt-check:
	@files="$$(gofmt -l $(GO_SOURCES))"; \
	if [ -n "$$files" ]; then \
		printf '以下 Go 文件需要 gofmt：\n%s\n' "$$files"; \
		exit 1; \
	fi

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

tidy-check:
	go mod tidy -diff

contracts:
	python3 scripts/check-contracts.py

repository-check:
	python3 scripts/check-repository.py

$(WEB_INSTALL_STAMP): web/package.json web/package-lock.json
	cd web && npm ci
	@touch $(WEB_INSTALL_STAMP)

web-deps: $(WEB_INSTALL_STAMP)

web-build: web-deps
	cd web && npm run build

check: fmt-check tidy-check test vet contracts repository-check

ci: web-build check test-race build-nexusdock

build: web-build check build-nexusdock

build-nexusdock:
	mkdir -p bin
	go build -trimpath -o bin/nexusdock ./cmd/nexusdock

run: run-nexusdock

run-nexusdock:
	go run ./cmd/nexusdock

clean:
	rm -rf bin web/node_modules web/.vite web/tsconfig.tsbuildinfo
