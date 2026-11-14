# Builds, checks and packages the rembox CLI. `make help` lists the targets.

PKG       := github.com/aakashns/rembox/cli/internal/version
# The release builds pass VERSION=$(cat VERSION). Other builds are 0.2.0-dev, which `rembox
# upgrade` treats as older than the 0.2.0 release.
VERSION   ?= $(shell cat VERSION)-dev
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null)
DATE      ?= $(shell git log -1 --format=%cI 2>/dev/null)
LDFLAGS   := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)
# The platforms each release has a binary for, as rembox-<os>-<arch>. Keep in sync with install.sh.
PLATFORMS := darwin/arm64 darwin/amd64 linux/arm64 linux/amd64
SHA256SUM := $(shell command -v sha256sum >/dev/null 2>&1 && echo sha256sum || echo shasum -a 256)

.PHONY: help build test lint dist clean

help: ## List the targets
	@grep -E '^[a-z]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{ printf "  %-8s %s\n", $$1, $$2 }'

build: ## Build bin/rembox for this machine
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/rembox ./cmd/rembox

test: ## Run the tests (including install.sh's, against a pretend release)
	go test -race ./...

lint: ## Check formatting, vet the code, and shellcheck install.sh, scripts/ and the server scripts
	@test -z "$$(gofmt -l .)" || { echo "Needs gofmt:"; gofmt -l .; exit 1; }
	go vet ./...
	shellcheck ../site/public/install.sh ../scripts/*.sh
	@# The server scripts are checked as rembox sends setup.sh: lib.sh, the steps, then setup.sh.
	cd internal/remote && for s in lib.sh $$(ls *.sh | grep -vx 'lib.sh\|setup.sh') setup.sh; do \
	  cat "$$s"; echo; done | shellcheck -s bash -

dist: ## Build every platform's binary and SHA256SUMS into dist/, as a release has them
	rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; \
	  echo "  dist/rembox-$$os-$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
	    -o dist/rembox-$$os-$$arch ./cmd/rembox || exit 1; \
	done
	cd dist && $(SHA256SUM) rembox-* > SHA256SUMS

clean: ## Remove build output
	rm -rf bin dist
