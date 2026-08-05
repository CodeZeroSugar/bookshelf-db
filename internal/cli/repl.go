package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"bookshelf-db/internal/export"
	"bookshelf-db/internal/models"
	"bookshelf-db/internal/store"
)

const prompt = "bookshelf> "

type App struct {
	st    *store.Store
	input *bufio.Scanner
	ctx   context.Context
}

func NewApp(st *store.Store) *App {
	return &App{st: st, input: bufio.NewScanner(os.Stdin), ctx: context.Background()}
}

func (a *App) Run() {
	fmt.Println("bookshelf-db  |  type 'help' for commands, 'exit' to quit")
	for {
		fmt.Print(prompt)
		if !a.input.Scan() {
			fmt.Println()
			return
		}
		line := strings.TrimSpace(a.input.Text())
		if line == "" {
			continue
		}
		if err := a.exec(line); err != nil {
			fmt.Println("error:", err)
		}
	}
}

func (a *App) exec(line string) error {
	cmd, rest := splitFirst(line)
	switch cmd {
	case "help", "?":
		printHelp()
	case "exit", "quit", "q":
		os.Exit(0)
	case "status":
		return a.cmdStatus()
	case "add-check":
		title, author := parseTitleAuthor(rest)
		if title == "" {
			return fmt.Errorf("usage: add-check <title> [| author]")
		}
		return a.cmdAdd("check_against", []models.Book{{Title: title, Author: author}})
	case "add-library":
		title, author := parseTitleAuthor(rest)
		if title == "" {
			return fmt.Errorf("usage: add-library <title> [| author]")
		}
		return a.cmdAdd("user_library", []models.Book{{Title: title, Author: author}})
	case "import-check":
		if rest == "" {
			return fmt.Errorf("usage: import-check <file.json | json>")
		}
		books, err := models.ParseBooks(rest)
		if err != nil {
			return err
		}
		return a.cmdAdd("check_against", books)
	case "import-library":
		if rest == "" {
			return fmt.Errorf("usage: import-library <file.json | json>")
		}
		books, err := models.ParseBooks(rest)
		if err != nil {
			return err
		}
		return a.cmdAdd("user_library", books)
	case "remove-check":
		if rest == "" {
			return fmt.Errorf("usage: remove-check <title>")
		}
		return a.cmdRemove("check_against", rest)
	case "remove-library":
		if rest == "" {
			return fmt.Errorf("usage: remove-library <title>")
		}
		return a.cmdRemove("user_library", rest)
	case "query-check":
		if rest == "" {
			return fmt.Errorf("usage: query-check <title>")
		}
		return a.cmdQuery("check_against", rest)
	case "query-library":
		if rest == "" {
			return fmt.Errorf("usage: query-library <title>")
		}
		return a.cmdQuery("user_library", rest)
	case "export-missing":
		return a.cmdExport("missing", rest)
	case "export-check":
		return a.cmdExport("check_against", rest)
	case "export-library":
		return a.cmdExport("user_library", rest)
	case "compare":
		return a.cmdCompare()
	case "matches":
		return a.cmdMatches()
	case "clear":
		return a.cmdClear(rest)
	default:
		return fmt.Errorf("unknown command %q (try 'help')", cmd)
	}
	return nil
}

func (a *App) cmdStatus() error {
	c, l, m, err := a.st.Count(a.ctx)
	if err != nil {
		return err
	}
	fmt.Printf("check against: %d | user library: %d | matches: %d\n", c, l, m)
	return nil
}

func (a *App) cmdAdd(table string, books []models.Book) error {
	var (
		res store.AddResult
		err error
	)
	if table == "check_against" {
		res, err = a.st.AddCheckAgainst(a.ctx, books)
	} else {
		res, err = a.st.AddLibrary(a.ctx, books)
	}
	if err != nil {
		return err
	}
	fmt.Printf("added %d, skipped %d (already present)\n", res.Added, res.Skipped)
	for _, h := range res.Hits {
		fmt.Printf("  MATCH: %q is in check-against list (%q)\n", h.LibraryTitle, h.CheckAuthor)
	}
	return nil
}

func (a *App) cmdRemove(table, title string) error {
	var (
		removed bool
		close   []models.Book
		err     error
	)
	if table == "check_against" {
		removed, close, err = a.st.RemoveCheckAgainst(a.ctx, title)
	} else {
		removed, close, err = a.st.RemoveLibrary(a.ctx, title)
	}
	if err != nil {
		return err
	}
	if removed {
		fmt.Printf("removed %q from %s\n", title, table)
		return nil
	}
	if len(close) == 0 {
		fmt.Printf("no exact match for %q and no close matches found\n", title)
		return nil
	}
	return a.pickAndRemove(table, title, close)
}

func (a *App) pickAndRemove(table, original string, close []models.Book) error {
	fmt.Printf("no exact match for %q. Close matches:\n", original)
	for i, b := range close {
		suffix := ""
		if b.Author != "" {
			suffix = " by " + b.Author
		}
		fmt.Printf("  %d) %s%s\n", i+1, b.Title, suffix)
	}
	fmt.Print("pick a number to remove, or 0 to cancel: ")
	if !a.input.Scan() {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(a.input.Text()))
	if err != nil || n < 1 || n > len(close) {
		fmt.Println("cancelled")
		return nil
	}
	pick := close[n-1]
	var removed bool
	if table == "check_against" {
		removed, _, err = a.st.RemoveCheckAgainst(a.ctx, pick.Title)
	} else {
		removed, _, err = a.st.RemoveLibrary(a.ctx, pick.Title)
	}
	if err != nil {
		return err
	}
	if removed {
		fmt.Printf("removed %q from %s\n", pick.Title, table)
	} else {
		fmt.Println("nothing removed")
	}
	return nil
}

func (a *App) cmdQuery(table, title string) error {
	book, found, close, err := a.st.Lookup(a.ctx, table, title)
	if err != nil {
		return err
	}
	label := map[string]string{"check_against": "check-against list", "user_library": "your library"}[table]
	if found {
		fmt.Printf("FOUND: %s\n", export.Display(book))
		return nil
	}
	if len(close) == 0 {
		fmt.Printf("not in %s: %q (no close matches)\n", label, title)
		return nil
	}
	fmt.Printf("not in %s: %q. Close matches:\n", label, title)
	for i, b := range close {
		fmt.Printf("  %d) %s\n", i+1, export.Display(b))
	}
	return nil
}

func (a *App) cmdExport(what, rest string) error {
	format, file := splitTwo(rest)
	var (
		books []models.Book
		err   error
	)
	switch what {
	case "missing":
		books, err = a.st.Missing(a.ctx)
	case "check_against":
		books, err = a.st.AllCheckAgainst(a.ctx)
	case "user_library":
		books, err = a.st.AllLibrary(a.ctx)
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
}

func (a *App) cmdCompare() error {
	ms, err := a.st.Compare(a.ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%d book(s) in your library are on the check-against list:\n", len(ms))
	for _, m := range ms {
		author := m.CheckAuthor
		if author == "" {
			author = m.LibraryAuthor
		}
		fmt.Printf("  - %s%s\n", m.CheckTitle, authorSuffix(author))
	}
	return nil
}

func (a *App) cmdMatches() error {
	ms, err := a.st.Matches(a.ctx)
	if err != nil {
		return err
	}
	if len(ms) == 0 {
		fmt.Println("no matches recorded yet (run 'compare')")
		return nil
	}
	for _, m := range ms {
		fmt.Printf("  library: %s%s  ->  check: %s%s\n",
			m.LibraryTitle, authorSuffix(m.LibraryAuthor),
			m.CheckTitle, authorSuffix(m.CheckAuthor))
	}
	return nil
}

func (a *App) cmdClear(scope string) error {
	switch scope {
	case "check", "library", "all":
	default:
		return fmt.Errorf("usage: clear check | library | all")
	}
	c, l, _, err := a.st.Count(a.ctx)
	if err != nil {
		return err
	}
	desc := map[string]string{
		"check":   fmt.Sprintf("%d row(s) from check_against", c),
		"library": fmt.Sprintf("%d row(s) from user_library", l),
		"all":     fmt.Sprintf("%d row(s) from check_against and %d from user_library", c, l),
	}[scope]
	fmt.Printf("This will delete %s.\nType 'yes' to confirm: ", desc)
	if !a.input.Scan() {
		return nil
	}
	if strings.TrimSpace(a.input.Text()) != "yes" {
		fmt.Println("cancelled")
		return nil
	}
	n, err := a.st.Clear(a.ctx, scope)
	if err != nil {
		return err
	}
	fmt.Printf("deleted %d row(s)\n", n)
	return nil
}

// splitFirst returns the first word and the remainder of the line.
func splitFirst(line string) (string, string) {
	line = strings.TrimSpace(line)
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return line, ""
	}
	return line[:idx], strings.TrimSpace(line[idx+1:])
}

// splitTwo returns the first two words ("format" and "file" for export).
func splitTwo(s string) (string, string) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}
	return fields[0], fields[1]
}

// parseTitleAuthor splits "Title | Author" on the pipe. Author is optional.
func parseTitleAuthor(s string) (string, string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "|"); i >= 0 {
		title := strings.TrimSpace(s[:i])
		author := strings.TrimSpace(s[i+1:])
		return title, author
	}
	return s, ""
}

func authorSuffix(a string) string {
	if a == "" {
		return ""
	}
	return " by " + a
}

func printHelp() {
	fmt.Println(`
commands:
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

export fmt: text (default), json, or csv. If file is omitted, output goes to stdout:
  export-missing csv missing.csv

Use ' | ' to separate title from an optional author:
  add-library The Old Truck | Jerome Pumphrey`)
}
