# Local development. The stack's ports are 127.0.0.1 only:
#   backpack 8780, FalkorDB 16379, Typesense 18108.
COMPOSE := docker compose -f deploy/docker-compose.yml
export TYPESENSE_API_KEY ?= local-dev-only

TEST_ENV := BACKPACK_TEST_FALKORDB=falkor://localhost:16379 \
            BACKPACK_TEST_TYPESENSE=http://localhost:18108 \
            BACKPACK_TEST_TYPESENSE_KEY=$(TYPESENSE_API_KEY)

# The lowest total test coverage, in percent, the gates accept.
COVERAGE_MIN := 70

.PHONY: up tunnel down logs test gates coverage token reindex

up:            ## start the whole stack, rebuilding the service
	$(COMPOSE) up -d --build

tunnel:        ## start the stack and the Cloudflare tunnel (needs CLOUDFLARE_TOKEN)
	@test -n "$$CLOUDFLARE_TOKEN" || (echo "tunnel: set CLOUDFLARE_TOKEN" && exit 1)
	$(COMPOSE) --profile tunnel up -d --build

down:          ## stop the stack (data volumes are kept)
	$(COMPOSE) --profile tunnel down

logs:
	$(COMPOSE) logs -f backpack

test:          ## all tests, against the local FalkorDB and Typesense
	$(TEST_ENV) go test -race ./...

gates:         ## every gate in docs/ENGINEERING-STANDARD.md §7.3
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "gofmt: files need formatting" && exit 1)
	go vet ./...
	$(TEST_ENV) go test -race -coverprofile=coverage.out ./...
	@$(MAKE) --no-print-directory coverage
	go build ./...

coverage:      ## fail if total coverage in coverage.out is below COVERAGE_MIN
	@go tool cover -func=coverage.out | awk -v min=$(COVERAGE_MIN) \
		'/^total:/ { sub("%", "", $$3); print "coverage: " $$3 "% (minimum " min "%)"; if ($$3 + 0 < min) exit 1 }'

token:         ## make token ACTOR=<name>
	@$(COMPOSE) exec backpack /backpack token create --actor $(ACTOR)

reindex:       ## rebuild every workspace's search index from the board
	@$(COMPOSE) exec backpack /backpack reindex
