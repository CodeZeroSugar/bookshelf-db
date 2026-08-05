package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBooksObject(t *testing.T) {
	books, err := ParseBooks(`{"title": "The Old Truck", "author": "Jerome and Jarrett Pumphrey"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "The Old Truck" || books[0].Author != "Jerome and Jarrett Pumphrey" {
		t.Fatalf("got %+v", books)
	}
}

func TestParseBooksArray(t *testing.T) {
	in := `[
		{"title": "Hush!", "author": "Minfong Ho"},
		{"title": "The Runaway Bunny", "author": "Margaret Wise Brown"}
	]`
	books, err := ParseBooks(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("want 2 books, got %d", len(books))
	}
	if books[1].Title != "The Runaway Bunny" {
		t.Fatalf("got %+v", books[1])
	}
}

func TestParseBooksAuthorOptional(t *testing.T) {
	books, err := ParseBooks(`{"title": "A Tree is Nice"}`)
	if err != nil {
		t.Fatal(err)
	}
	if books[0].Author != "" {
		t.Fatalf("want empty author, got %q", books[0].Author)
	}
}

func TestParseBooksFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "books.json")
	if err := os.WriteFile(path, []byte(`[{"title": "Swimmy", "author": "Leo Lionni"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	books, err := ParseBooks(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "Swimmy" {
		t.Fatalf("got %+v", books)
	}
}

func TestParseBooksInvalid(t *testing.T) {
	cases := []string{
		`not json`,
		`{"title": 123}`,
		`{"title": "X", "author": 42}`,
		`{}`,
		`{"author": "Minfong Ho"}`,
		`{"title": "   "}`,
		`{"title": null}`,
		`{"title": "X", "year": 1999}`,
		`[{"title": "X"}, {"author": "Who"}]`,
		`[{"title": "X"}, {"title": "Y", "extra": true}]`,
		`{"title": "X"} trailing`,
		`[{"title": "X"}][{"title": "Y"}]`,
		`{"title": "X", "author": "A", "unknown": 1}`,
	}
	for _, in := range cases {
		if _, err := ParseBooks(in); err == nil {
			t.Errorf("ParseBooks(%q) = nil error, want rejection", in)
		}
	}
	if _, err := ParseBooks(`[]`); err != nil {
		t.Fatalf("empty array should be valid: %v", err)
	}
}

func TestParseBooksNullAuthorValid(t *testing.T) {
	books, err := ParseBooks(`{"title": "Hush!", "author": null}`)
	if err != nil {
		t.Fatal(err)
	}
	if books[0].Author != "" {
		t.Fatalf("want empty author for null, got %q", books[0].Author)
	}
}

func TestParseBooksErrorNamesEntry(t *testing.T) {
	_, err := ParseBooks(`[{"title": "One"}, {"title": "   "}, {"title": "Three"}]`)
	if err == nil {
		t.Fatal("want error")
	}
	want := "entry 2"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q should name %q", err, want)
	}
}

func TestParseBooksErrorNamesUnknownKey(t *testing.T) {
	_, err := ParseBooks(`{"title": "X", "year": 1999}`)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "year") {
		t.Fatalf("error %q should mention offending key", err)
	}
}
