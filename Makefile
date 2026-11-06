GO ?= go
UVX ?= uvx
FUZZTIME ?= 10s
FUZZPARALLEL ?= 4
MUTANTS_WORKERS ?= 4

TOOL := $(GO) tool -modfile=tools/go.mod
ACTIONLINT := $(GO) tool -modfile=tools/actionlint/go.mod actionlint
GREMLINS := $(GO) tool -modfile=tools/gremlins/go.mod gremlins
TOOL_MODULES := tools tools/actionlint tools/gremlins
pinned = $(shell sed -n 's/^$(1)==//p' tools/requirements.txt)
ZIZMOR := $(UVX) zizmor@$(call pinned,zizmor)

RELEASE_TAG := v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc[1-9][0-9]*)?

COVER_DIR := $(CURDIR)/.cover
# COVER_MIN = total of 2026-11-06 (99.8%) rounded down
COVER_MIN := 99
MUTANTS_WORK := $(CURDIR)/.mutants

.DEFAULT_GOAL := build

.PHONY: check build test cover fuzz lint tidy vuln mutants release-gate examples

check: lint tidy test cover fuzz vuln build

build:
	$(GO) build -o primitive.bin .

test:
	GORACE=atexit_sleep_ms=0 $(GO) test -race ./...

cover:
	rm -rf $(COVER_DIR) && mkdir -p $(COVER_DIR)
	$(GO) test -covermode=atomic -coverpkg=./... -coverprofile=$(COVER_DIR)/cover.out ./...
	$(GO) tool cover -html=$(COVER_DIR)/cover.out -o $(COVER_DIR)/cover.html
	@$(GO) tool cover -func=$(COVER_DIR)/cover.out | awk -v min=$(COVER_MIN) '/^total:/ { seen = 1; sub("%", "", $$NF); \
		print "statements covered: " $$NF "%, floor " min "%"; below = $$NF + 0 < min } \
		END { exit !seen || below }'

fuzz:
	listed=$$($(GO) test ./primitive -list '^Fuzz') || exit 1; \
	for target in $$(echo "$$listed" | grep '^Fuzz'); do \
		$(GO) test ./primitive -run '^$$' -fuzz "^$$target$$" -fuzztime $(FUZZTIME) -parallel $(FUZZPARALLEL) || exit 1; \
	done

lint:
	$(TOOL) golangci-lint run ./...
	$(ACTIONLINT)
	$(ZIZMOR) --offline .

tidy:
	$(GO) mod tidy -diff
	for dir in $(TOOL_MODULES); do (cd $$dir && $(GO) mod tidy -diff) || exit 1; done

vuln:
	$(TOOL) govulncheck ./...

mutants:
	@case "$${BASE-origin/master}" in -*|*[!0-9A-Za-z./_~^-]*) echo "invalid BASE: $$BASE" >&2; exit 1;; esac
	rm -rf $(MUTANTS_WORK) && mkdir -p $(MUTANTS_WORK)/src
	git ls-files -z -co --exclude-standard | tar --null -T - -cf - | tar -C $(MUTANTS_WORK)/src -xf -
	base=$${BASE-origin/master}; cd $(MUTANTS_WORK)/src && GIT_DIR=$(CURDIR)/.git GIT_WORK_TREE=$(MUTANTS_WORK)/src GIT_OPTIONAL_LOCKS=0 \
		$(GREMLINS) unleash --config .gremlins.yaml --workers $(MUTANTS_WORKERS) \
		$${base:+--diff "$$base"} -o $(MUTANTS_WORK)/report.json

examples: build
	mkdir -p examples/output
	for image in lenna monalisa owl pyramids; do \
		for mode in 1 3 7; do \
			./primitive.bin -i examples/$$image.png -o examples/output/$$image-m$$mode.svg -n 200 -m $$mode -seed 1 -j 4; \
		done; \
	done
	./primitive.bin -i examples/monalisa.png -o examples/output/monalisa.gif -n 100 -s 512 -seed 1 -j 4

release-gate:
	@case "$$TAG" in *[!0-9A-Za-z.-]*|'') echo "invalid tag: $$TAG" >&2; exit 1;; esac
	@echo "$$TAG" | grep -Eqx '$(RELEASE_TAG)' || { echo "invalid tag: $$TAG" >&2; exit 1; }
	@commit=$$(git rev-parse "$$TAG^{commit}"); \
	test "$$commit" = "$$(git rev-parse origin/master)" || { echo "$$TAG is not the head of master" >&2; exit 1; }; \
	runs=$$(gh run list --workflow ci.yml --commit "$$commit" --branch master --event push --status success --json databaseId --jq length); \
	test "$$runs" -gt 0 || { echo "no successful ci.yml run for $$commit" >&2; exit 1; }
