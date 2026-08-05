package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"bookshelf-db/internal/cli"
	"bookshelf-db/internal/db"
	"bookshelf-db/internal/export"
	"bookshelf-db/internal/models"
	"bookshelf-db/internal/store"
)

func main() {
	ctx := context.Background()
	args := os.Args[1:]

	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		usage()
		return
	}

	if len(args) == 0 {
		runREPL(ctx)
		return
	}

	if err := runOneOff(ctx, args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runREPL(ctx context.Context) {
	pool, err := db.Connect(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		fmt.Fprintln(os.Stderr, "hint: is the database running? (docker compose up -d)")
		os.Exit(1)
	}
	defer pool.Close()
	warnPendingMigrations(ctx, pool)
	cli.NewApp(store.New(pool)).Run()
}

// warnPendingMigrations alerts the user when the database schema is behind the
// binary but never applies migrations implicitly.
func warnPendingMigrations(ctx context.Context, pool *pgxpool.Pool) {
	mig, err := db.NewMigrator(ctx, pool)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: cannot check migration status:", err)
		return
	}
	defer mig.Close()
	pending, err := mig.HasPending(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: cannot check migration status:", err)
		return
	}
	if pending {
		fmt.Fprintln(os.Stderr,
			"warning: database schema has pending migrations (schema changed since your last setup).\n"+
				"         run 'bookshelf migrate up' or 'make migrate' to apply them.")
	}
}

func runOneOff(ctx context.Context, args []string) error {
	pool, err := db.Connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch args[0] {
	case "init":
		return cmdMigrateUp(ctx, pool, false)

	case "migrate":
		if len(args) < 2 {
			return fmt.Errorf("usage: bookshelf migrate up | status")
		}
		switch args[1] {
		case "up":
			force := len(args) > 2 && args[2] == "--force"
			return cmdMigrateUp(ctx, pool, force)
		case "status":
			return cmdMigrateStatus(ctx, pool)
		default:
			return fmt.Errorf("usage: bookshelf migrate up | status")
		}

	case "import-check":
		if len(args) < 2 {
			return fmt.Errorf("usage: bookshelf import-check <file.json|json>")
		}
		books, err := models.ParseBooks(args[1])
		if err != nil {
			return err
		}
		res, err := store.New(pool).AddCheckAgainst(ctx, books)
		if err != nil {
			return err
		}
		fmt.Printf("check_against: added %d, skipped %d\n", res.Added, res.Skipped)
		return nil

	case "import-library":
		if len(args) < 2 {
			return fmt.Errorf("usage: bookshelf import-library <file.json|json>")
		}
		books, err := models.ParseBooks(args[1])
		if err != nil {
			return err
		}
		res, err := store.New(pool).AddLibrary(ctx, books)
		if err != nil {
			return err
		}
		fmt.Printf("user_library: added %d, skipped %d\n", res.Added, res.Skipped)
		for _, h := range res.Hits {
			fmt.Printf("  MATCH: %q is in check-against list (%q)\n", h.LibraryTitle, h.CheckAuthor)
		}
		return nil

	case "query-check", "query-library":
		if len(args) < 2 {
			return fmt.Errorf("usage: bookshelf %s <title>", args[0])
		}
		table := map[string]string{"query-check": "check_against", "query-library": "user_library"}[args[0]]
		book, found, close, err := store.New(pool).Lookup(ctx, table, args[1])
		if err != nil {
			return err
		}
		if found {
			fmt.Println("FOUND:", export.Display(book))
			return nil
		}
		if len(close) == 0 {
			fmt.Printf("not found: %q (no close matches)\n", args[1])
			return nil
		}
		fmt.Printf("not found: %q. Close matches:\n", args[1])
		for i, b := range close {
			fmt.Printf("  %d) %s\n", i+1, export.Display(b))
		}
		return nil

	case "export-missing", "export-check", "export-library":
		if len(args) < 2 {
			return fmt.Errorf("usage: bookshelf %s <text|json|csv> [file]", args[0])
		}
		format, file := args[1], ""
		if len(args) > 2 {
			file = args[2]
		}
		st := store.New(pool)
		var books []models.Book
		switch args[0] {
		case "export-missing":
			books, err = st.Missing(ctx)
		case "export-check":
			books, err = st.AllCheckAgainst(ctx)
		case "export-library":
			books, err = st.AllLibrary(ctx)
		}
		if err != nil {
			return err
		}
		if file == "" {
			return export.Write(os.Stdout, format, books)
		}
		f, err := os.Create(file)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := export.Write(f, format, books); err != nil {
			return err
		}
		fmt.Printf("wrote %d book(s) to %s (%s)\n", len(books), file, format)
		return nil

	default:
		return fmt.Errorf("unknown subcommand %q\n\nuse one of: init, migrate, import-check, import-library, query-check, query-library, export-missing, export-check, export-library, or run bare 'bookshelf' for the interactive shell", args[0])
	}
}

func cmdMigrateUp(ctx context.Context, pool *pgxpool.Pool, force bool) error {
	mig, err := db.NewMigrator(ctx, pool)
	if err != nil {
		return err
	}
	defer mig.Close()
	versions, err := mig.Up(ctx, force)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		fmt.Println("database schema is up to date")
		return nil
	}
	statuses, err := mig.Status(ctx)
	if err != nil {
		return err
	}
	applied := map[int64]string{}
	for _, s := range statuses {
		if !s.Pending {
			applied[s.Version] = s.File
		}
	}
	for _, v := range versions {
		fmt.Printf("applied %s\n", applied[v])
	}
	return nil
}

func cmdMigrateStatus(ctx context.Context, pool *pgxpool.Pool) error {
	mig, err := db.NewMigrator(ctx, pool)
	if err != nil {
		return err
	}
	defer mig.Close()
	statuses, err := mig.Status(ctx)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("no migrations found")
		return nil
	}
	for _, s := range statuses {
		state := "pending"
		if !s.Pending {
			state = "applied " + s.AppliedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-40s %s\n", s.File, state)
	}
	return nil
}

func usage() {
	fmt.Println(`bookshelf-db — check your library against a "books to check against" list.

usage:
  bookshelf                    interactive shell
  bookshelf init               apply all migrations (idempotent)
  bookshelf migrate up         apply pending migrations (additive-only guard)
  bookshelf migrate status     show applied vs pending migrations
  bookshelf import-check F     add books from a .json file to check-against list
  bookshelf import-library F   add books from a .json file to your library
  bookshelf query-check T      look up a title in the check-against list
  bookshelf query-library T    look up a title in your library
  bookshelf export-missing F   export check-against books you don't own
  bookshelf export-check F     export the check-against list
  bookshelf export-library F   export your library
                               (F is one of: text, json, csv; optional 2nd arg = file)

config:
  DATABASE_URL  postgres connection string
                (default postgres://bookshelf:bookshelf@localhost:5432/bookshelf)

json format:
  a single {"title":"...","author":"..."} object or an array of them`)
}
