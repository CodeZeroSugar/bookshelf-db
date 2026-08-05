# bookshelf-db

Check what books you already own against a list of books you want to avoid
buying twice. Books are loaded into a Postgres database from `.json` files,
and a command-line shell lets you add to your library, query both lists, run
overlap comparisons, and export reports.

## Features

- Load a "books to check against" list from `.json` files
- Add your own books to a `user_library` table (author optional)
- Instant notification when a book you add is on the check-against list
- Case-insensitive, punctuation-tolerant matching (`hush!` matches `Hush!`)
- Close-match suggestions for typos and title variants
- `compare` reports which of your books are on the check-against list and
  records them in a `matches` table
- `missing` report: check-against books you don't own yet
- Export any list (library, check-against, missing) as text, JSON, or CSV
- Strict JSON validation when ingesting data

## Requirements

- [Go](https://go.dev/dl/) 1.26+
- [Docker](https://www.docker.com/) (for the Postgres container)

No other setup is required — the Postgres server runs in Docker and the schema
is created automatically.

## Quickstart

```sh
git clone  https://github.com/CodeZeroSugar/bookshelf-db && cd bookshelf-db
make setup        # starts both Postgres (prod + dev), builds, migrates, asks about sample data
make run          # start the interactive shell (against production)
```

`make setup` will ask whether to seed the sample check list from
`data/books.json` (42 books) into the production database. Say `y` to try the
app with sample data, or `n` to start empty — you can always load it later
with `make seed`.

## Configuration

Connection is controlled by the `DATABASE_URL` environment variable:

```
DATABASE_URL=postgres://bookshelf:bookshelf@localhost:5432/bookshelf
```

The default above matches the bundled `docker-compose.yml`, so most users need
to change nothing. To point at a different database:

```sh
DATABASE_URL=postgres://user:pass@host:5432/dbname ./bookshelf
```

Copy `.env.example` to `.env` to set `DATABASE_URL` and `BOOKSHELF_DEV_URL`
persistently; the Makefile picks the file up automatically.

## Protecting your data (dev vs prod)

The repository runs **two separate Postgres instances**:

- **Production** (`db`, port `5432`, database `bookshelf`) — your real data.
  The app defaults to this for day-to-day use (`make run`, imports, queries,
  seeds, backups).
- **Development** (`db-dev`, port `5433`, database `bookshelf_dev`, its own
  volume) — used only by development tooling.

| Command                | Target | Notes                                        |
| ---------------------- | ------ | -------------------------------------------- |
| `make run`             | prod   | interactive shell                            |
| `make seed`            | prod   | loads sample check list                      |
| `make backup`          | prod   | pg_dump to `backups/`                        |
| `make migrate-prod`    | prod   | **backs up, then requires typing `yes`**     |
| `make setup`           | both   | migrates dev + prod, prompts to seed prod    |
| `make migrate` / `init`| dev    | migrations apply to dev only                 |
| `make migrate-status`  | dev    |                                              |
| `make test`            | dev    | throwaway databases on dev                  |
| `make run-dev`         | dev    | interactive shell against dev               |
| `make db-reset`        | both   | **requires typing `db-reset` to confirm**    |
| `make db-dev-reset`    | dev    | wipes dev volume, no confirmation            |

Safety rules that keep your production data intact:

- **Tests never touch production.** Integration tests connect to
  `BOOKSHELF_TEST_URL` (defaults to the dev database), only run against
  localhost hosts, and refuse to run if the target equals `DATABASE_URL`.
  They create and drop throwaway `bookshelf_mig_*` databases on dev.
- **Migrations apply to dev by default.** Applying a schema change to
  production is a deliberate, confirmed step: `make migrate-prod` runs a
  backup first and requires you to type `yes`, and the additive-only guard
  refuses destructive SQL (`DROP`/`TRUNCATE`/`DELETE FROM`) unless you pass
  `--force`.
- **`make db-reset` will not run silently.** It deletes **all** data in both
  volumes and requires you to type `db-reset` to confirm.
- **Back up before changing production schema**: `make backup` then
  `make migrate-prod`.

## Usage

### Interactive shell

Run `bookshelf` with no arguments (or `make run`). Type `help` for the full
list:

```
add-check <title> [| author]     add a book to the check-against list
add-library <title> [| author]   add a book to your library (reports hits)
import-check <file.json|json>    bulk add to check-against list
import-library <file.json|json>  bulk add to your library (reports hits)
remove-check <title>             remove from check-against list
remove-library <title>           remove from your library
query-check <title>              check a title against the check-against list
query-library <title>            check if a title is in your library
export-missing [fmt] [file]      list check-against books you don't own
export-check [fmt] [file]        export the check-against list
export-library [fmt] [file]      export your library
compare                          report & record library/check-against overlaps
matches                          list recorded overlap rows
status                           show row counts
clear check|library|all          delete rows (asks for confirmation)
help                             this help
exit                             quit
```

Separate an optional author with a pipe: `add-library The Old Truck | Jerome
Pumphrey`. Without a pipe the author is left empty.

### One-off commands

`bookshelf` also accepts subcommands for scripting:

```
bookshelf init                       apply all migrations (idempotent)
bookshelf migrate up                 apply pending migrations
bookshelf migrate status             show applied vs pending migrations
bookshelf import-check <file.json>   load a check-against file
bookshelf import-library <file.json> load library books from a file
bookshelf query-check <title>        look up a title
bookshelf query-library <title>      look up a title
bookshelf export-missing <fmt>       export to stdout (text|json|csv)
bookshelf export-check <fmt> [file]  export the check-against list
bookshelf export-library <fmt> [file] export your library
```

For exports, `fmt` is `text`, `json`, or `csv`. With a file argument the list
is written to the file; without one it prints to stdout.

## Upgrading

Your data lives in the production Postgres Docker volume, so updating the
application code never touches it. Schema changes are handled by versioned
migrations (tracked in the `goose_db_version` table), applied forward-only and
in order. Upgrading production is:

```sh
git pull
make build
make backup                # optional but recommended safety net
make migrate-prod          # backs up again, then requires you to type 'yes'
make run
```

You can preview a change against the dev database first with `make migrate`,
which never touches production.

Migrations never run automatically. The interactive shell only warns you when
the database is behind the binary:

```
warning: database schema has pending migrations (schema changed since your last setup).
         run 'bookshelf migrate up' to apply them (see make migrate / make migrate-prod).
```

New migrations must be additive-only (new tables/columns/indexes). A migration
containing destructive statements (`DROP`, `TRUNCATE`, `DELETE FROM`) is
refused unless you explicitly run `bookshelf migrate up --force`. To restore a
backup: `psql -f backups/bookshelf_<timestamp>.sql`.

## JSON input format

Files are strict. Each entry needs a non-empty `title`; `author` is optional.
Unknown keys are rejected, so a file with a typo like `"titel"` fails as a
whole (nothing is inserted).

Valid:

```json
[
  { "title": "The Old Truck", "author": "Jerome and Jarrett Pumphrey" },
  { "title": "Swirl by Swirl", "author": "Joyce Sidman" },
  { "title": "No Author Listed" }
]
```

A single object `{ "title": "...", "author": "..." }` is also accepted.

## Ingesting documents

Python scripts that parse PDFs, spreadsheets, etc. should always emit this
same `.json` shape so the Go binary has one normalized format:

```
python3 parse_catalog.py catalog.pdf > catalog.json
bookshelf import-check catalog.json
```

## Development

```sh
make all             # vet + test + build
make test            # run unit tests (integration tests use throwaway DBs on dev)
make fmt             # check formatting
make db-up           # start both postgres containers (prod 5432, dev 5433)
make migrate         # apply pending migrations to the DEV database
make migrate-status  # show applied vs pending migrations (dev)
make migrate-prod    # backup + apply migrations to production (requires confirmation)
make backup          # pg_dump production to backups/
make db-dev-reset    # wipe only the dev volume (safe)
make db-reset        # wipe both volumes (requires typing 'db-reset' to confirm)
```

Note: integration tests require the dev database to be running (`make db-up`);
they skip silently if it is unreachable.

## Project layout

```
main.go              subcommand dispatch + entry point
Makefile             build/setup/run targets (dev/prod split)
docker-compose.yml   two Postgres instances: db (prod 5432), db-dev (5433)
.env.example         documents DATABASE_URL and BOOKSHELF_DEV_URL
internal/models/     Book struct + strict JSON parsing
internal/db/         connection + versioned migrations (embedded SQL)
internal/store/      all database operations
internal/match/      title normalization
internal/export/     text/json/csv writers
internal/cli/        interactive shell
data/books.json      sample check-against list
```

## Troubleshooting

- **`error: ping ... no response` / connection refused** — Postgres isn't
  running. Start it with `make db-up`.
- **Port 5432 already in use** — stop the other Postgres or edit the port
  mapping in `docker-compose.yml` (and update `DATABASE_URL` to match).
- **`unknown field ...` on import** — your `.json` has a key the app doesn't
  recognize (`title` and `author` only).
- **`entry N: title is required`** — entry N is missing its title; the whole
  file was rejected.
