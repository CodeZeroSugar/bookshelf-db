package export

import (
	"bytes"
	"strings"
	"testing"

	"bookshelf-db/internal/models"
)

func testBooks() []models.Book {
	return []models.Book{
		{Title: "Hush!", Author: "Minfong Ho"},
		{Title: "A Tree is Nice", Author: ""},
		{Title: `Weird "Title", With Commas`, Author: `Author, "Quoted"`},
	}
}

func TestWriteText(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, "text", testBooks()); err != nil {
		t.Fatal(err)
	}
	want := "Hush! by Minfong Ho\nA Tree is Nice\nWeird \"Title\", With Commas by Author, \"Quoted\"\n"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, "json", testBooks()); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{`"title": "Hush!"`, `"author": "Minfong Ho"`, `"A Tree is Nice"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("json missing %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "[") {
		t.Fatalf("json should be an array:\n%s", got)
	}
}

func TestWriteCSVQuoting(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, "csv", testBooks()); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	want := "title,author\n" +
		"Hush!,Minfong Ho\n" +
		"A Tree is Nice,\n" +
		"\"Weird \"\"Title\"\", With Commas\",\"Author, \"\"Quoted\"\"\"\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteUnknownFormat(t *testing.T) {
	if err := Write(&bytes.Buffer{}, "xml", testBooks()); err == nil {
		t.Fatal("want error for unknown format")
	}
}
