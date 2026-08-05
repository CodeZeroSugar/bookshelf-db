BIN        := bookshelf
SEED       := data/books.json
BACKUP_DIR := backups

# Load optional .env overrides for DATABASE_URL / BOOKSHELF_DEV_URL
-include .env

# Production database — what the app uses day to day (REPL, imports, queries).
DATABASE_URL      ?= postgres://bookshelf:bookshelf@localhost:5432/bookshelf
# Dedicated dev/test database. Dev migrations and integration tests run here
# so the production database is never touched by development.
BOOKSHELF_DEV_URL ?= postgres://bookshelf:bookshelf@localhost:5433/bookshelf_dev

export DATABASE_URL BOOKSHELF_DEV_URL

.DEFAULT_GOAL := help
.PHONY: help db-up db-down db-logs db-logs-dev db-reset db-dev-reset build \
        test vet fmt clean all init seed run run-dev setup migrate \
        migrate-status migrate-prod backup

help: ## Show available targets
	@echo "bookshelf-db targets (PRODUCTION = $(DATABASE_URL)):"
	@echo "  dev = $(BOOKSHELF_DEV_URL)"
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

# --- databases -------------------------------------------------------------

db-up: ## Start both postgres containers (prod 5432, dev 5433) and wait
	docker compose up -d
	@echo "waiting for postgres (production)..."
	@i=0; until docker compose exec -T db pg_isready -U bookshelf -d bookshelf </dev/null >/dev/null 2>&1; do \
		i=$$((i+1)); if [ $$i -ge 30 ]; then echo "error: production postgres did not become ready"; exit 1; fi; \
		sleep 1; \
	done
	@echo "waiting for postgres (dev)..."
	@i=0; until docker compose exec -T db-dev pg_isready -U bookshelf -d bookshelf_dev </dev/null >/dev/null 2>&1; do \
		i=$$((i+1)); if [ $$i -ge 30 ]; then echo "error: dev postgres did not become ready"; exit 1; fi; \
		sleep 1; \
	done
	@echo "both databases ready"

db-down: ## Stop both postgres containers
	docker compose down

db-logs: ## Follow production postgres logs
	docker compose logs -f db

db-logs-dev: ## Follow dev postgres logs
	docker compose logs -f db-dev

db-reset: ## Wipe BOTH postgres volumes (requires typing 'db-reset' to confirm)
	@printf "This permanently deletes ALL data in the production AND dev volumes.\nType 'db-reset' to confirm: "; \
	read ans; \
	case "$$ans" in \
		db-reset) docker compose down -v ;; \
		*) echo "cancelled" ;; \
	esac

db-dev-reset: ## Wipe only the dev volume (safe, no confirmation needed)
	docker compose rm -sf db-dev
	docker volume rm -f bookshelf_pgdata_dev
	docker compose up -d db-dev

# --- build / test ----------------------------------------------------------

build: ## Build the bookshelf binary
	go build -o $(BIN) .

test: ## Run tests; integration tests use throwaway DBs on the dev database
	BOOKSHELF_TEST_URL=$(BOOKSHELF_DEV_URL) go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting (gofmt)
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "files need formatting:"; echo "$$out"; exit 1; fi; \
	echo "gofmt clean"

clean: ## Remove the built binary
	rm -f $(BIN)

all: vet test build ## Vet, test, and build

# --- migrations (dev by default) -------------------------------------------

init: build ## Apply all migrations to the DEV database
	DATABASE_URL=$(BOOKSHELF_DEV_URL) ./$(BIN) init

migrate: build ## Apply pending migrations to the DEV database
	DATABASE_URL=$(BOOKSHELF_DEV_URL) ./$(BIN) migrate up

migrate-status: build ## Show applied vs pending migrations (dev)
	DATABASE_URL=$(BOOKSHELF_DEV_URL) ./$(BIN) migrate status

migrate-prod: build backup ## Apply pending migrations to PRODUCTION (backup first, confirmation required)
	@printf "Applying migrations to the PRODUCTION database: $(DATABASE_URL)\nType 'yes' to continue: "; \
	read ans; \
	case "$$ans" in \
		[yY][eE][sS]) ./$(BIN) migrate up ;; \
		*) echo "cancelled" ;; \
	esac

backup: ## Dump the PRODUCTION database to backups/ (restore: psql -f backups/<file>)
	@mkdir -p $(BACKUP_DIR)
	@stamp=$$(date +%Y%m%d_%H%M%S); \
	docker compose exec -T db pg_dump -U bookshelf -d bookshelf </dev/null > $(BACKUP_DIR)/bookshelf_$$stamp.sql; \
	echo "backed up to $(BACKUP_DIR)/bookshelf_$$stamp.sql"

# --- app (production by default) -------------------------------------------

seed: build ## Load the sample check list into PRODUCTION (data/books.json)
	./$(BIN) import-check $(SEED)

run: build ## Start the interactive shell against PRODUCTION
	./$(BIN)

run-dev: build ## Start the interactive shell against the DEV database
	DATABASE_URL=$(BOOKSHELF_DEV_URL) ./$(BIN)

setup: db-up build init migrate ## Start dbs, build, migrate dev+prod, ask about seeding prod
	DATABASE_URL=$(DATABASE_URL) ./$(BIN) migrate up
	@printf "Seed sample check list ($(SEED)) into the PRODUCTION database? [y/N]: "; \
	read ans; \
	case "$$ans" in \
		[yY]|[yY][eE][sS]) $(MAKE) seed ;; \
		*) echo "skipped seeding (run 'make seed' later if you want it)" ;; \
	esac
