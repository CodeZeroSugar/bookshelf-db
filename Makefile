BIN      := bookshelf
SEED     := data/books.json
DB_URL   ?= postgres://bookshelf:bookshelf@localhost:5432/bookshelf
BACKUP_DIR := backups

.DEFAULT_GOAL := help
.PHONY: help db-up db-down db-logs db-reset build test vet fmt clean all \
        init seed run setup migrate migrate-status backup

help: ## Show available targets
	@echo "bookshelf-db targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

db-up: ## Start postgres in docker and wait until it accepts connections
	docker compose up -d
	@echo "waiting for postgres to be ready..."
	@i=0; until docker compose exec -T db pg_isready -U bookshelf -d bookshelf </dev/null >/dev/null 2>&1; do \
		i=$$((i+1)); if [ $$i -ge 30 ]; then echo "error: postgres did not become ready"; exit 1; fi; \
		sleep 1; \
	done
	@echo "postgres is ready"

db-down: ## Stop the postgres container
	docker compose down

db-logs: ## Follow postgres container logs
	docker compose logs -f db

db-reset: ## Stop postgres and delete its data volume (fresh state)
	docker compose down -v

build: ## Build the bookshelf binary
	go build -o $(BIN) .

test: ## Run the Go test suite
	go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting (gofmt)
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "files need formatting:"; echo "$$out"; exit 1; fi; \
	echo "gofmt clean"

clean: ## Remove the built binary
	rm -f $(BIN)

all: vet test build ## Vet, test, and build

init: build ## Apply all migrations (idempotent)
	DATABASE_URL=$(DB_URL) ./$(BIN) init

migrate: build ## Apply pending migrations (additive-only guard)
	DATABASE_URL=$(DB_URL) ./$(BIN) migrate up

migrate-status: build ## Show applied vs pending migrations
	DATABASE_URL=$(DB_URL) ./$(BIN) migrate status

backup: ## Dump the database to backups/ (restore with: psql -f backups/<file>)
	@mkdir -p $(BACKUP_DIR)
	@stamp=$$(date +%Y%m%d_%H%M%S); \
	docker compose exec -T db pg_dump -U bookshelf -d bookshelf > $(BACKUP_DIR)/bookshelf_$$stamp.sql; \
	echo "backed up to $(BACKUP_DIR)/bookshelf_$$stamp.sql"

seed: build ## Load the sample check list (data/books.json)
	DATABASE_URL=$(DB_URL) ./$(BIN) import-check $(SEED)

run: build ## Start the interactive shell
	DATABASE_URL=$(DB_URL) ./$(BIN)

setup: db-up build init ## Start db, build, init; then ask about seeding
	@printf "Seed sample check list ($(SEED))? [y/N]: "; \
	read ans; \
	case "$$ans" in \
		[yY]|[yY][eE][sS]) $(MAKE) seed ;; \
		*) echo "skipped seeding (run 'make seed' later if you want it)" ;; \
	esac
