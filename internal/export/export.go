package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"

	"bookshelf-db/internal/models"
)

// Write renders books in the given format ("text", "json", or "csv") to w.
func Write(w io.Writer, format string, books []models.Book) error {
	switch format {
	case "text", "":
		return writeText(w, books)
	case "json":
		return writeJSON(w, books)
	case "csv":
		return writeCSV(w, books)
	default:
		return fmt.Errorf("unknown format %q (want text, json, or csv)", format)
	}
}

func writeText(w io.Writer, books []models.Book) error {
	for _, b := range books {
		if _, err := fmt.Fprintln(w, display(b)); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w io.Writer, books []models.Book) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(books)
}

func writeCSV(w io.Writer, books []models.Book) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"title", "author"}); err != nil {
		return err
	}
	for _, b := range books {
		if err := cw.Write([]string{b.Title, b.Author}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Display renders a single book as "Title by Author" (author omitted when empty).
func Display(b models.Book) string {
	return display(b)
}

func display(b models.Book) string {
	if b.Author == "" {
		return b.Title
	}
	return b.Title + " by " + b.Author
}
