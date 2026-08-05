package main

import (
	"context"
	"fmt"
	"os"

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
	if err := db.Migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cli.NewApp(store.New(pool)).Run()
}

func runOneOff(ctx context.Context, args []string) error {
	pool, err := db.Connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch args[0] {
	case "init":
		if err := db.Migrate(ctx, pool); err != nil {
			return err
		}
		fmt.Println("schema ready (check_against, user_library, matches)")
		return nil

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
		return fmt.Errorf("unknown subcommand %q\n\nuse one of: init, import-check, import-library, query-check, query-library, export-missing, export-check, export-library, or run bare 'bookshelf' for the interactive shell", args[0])
	}
}

func usage() {
	fmt.Println(`bookshelf-db — check your library against a "books to check against" list.

usage:
  bookshelf                    interactive shell
  bookshelf init               create tables (idempotent)
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
