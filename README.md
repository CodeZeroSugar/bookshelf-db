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
git clone <your-clone-url> && cd bookshelf-db
make setup        # starts Postgres, builds, creates tables, asks about sample data
make run          # start the interactive shell
```

`make setup` will ask whether to seed the sample check list from
`data/books.json` (42 books). Say `y` to try the app with sample data, or `n`
to start empty — you can always load it later with `make seed`.

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
bookshelf init                       create tables (idempotent)
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
make all      # vet + test + build
make test     # run unit tests
make fmt      # check formatting
make db-reset # wipe the postgres data volume for a clean slate
```

## Project layout

```
main.go              subcommand dispatch + entry point
Makefile             build/setup/run targets
docker-compose.yml   local Postgres
internal/models/     Book struct + strict JSON parsing
internal/db/         connection + schema (embedded)
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
