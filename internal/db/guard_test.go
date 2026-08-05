package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpSection(t *testing.T) {
	content := "-- +goose Up\nCREATE TABLE x();\n\n-- +goose Down\nDROP TABLE x();\n"
	got := upSection(content)
	want := "-- +goose Up\nCREATE TABLE x();\n\n"
	if got != want {
		t.Fatalf("upSection got %q want %q", got, want)
	}

	if got := upSection("no markers"); got != "no markers" {
		t.Fatalf("upSection without markers got %q", got)
	}
}

func TestDestructiveRe(t *testing.T) {
	allowed := []string{
		"CREATE TABLE foo (id INT);",
		"ALTER TABLE foo ADD COLUMN bar TEXT;",
		"CREATE INDEX idx ON foo (bar);",
		"CREATE EXTENSION IF NOT EXISTS pg_trgm;",
		"UPDATE goose_version SET x = 1;", // not matched by policy
	}
	for _, s := range allowed {
		if destructiveRE.MatchString(s) {
			t.Errorf("destructiveRE should NOT match: %q", s)
		}
	}

	blocked := []string{
		"DROP TABLE foo;",
		"drop index if exists idx;",
		"ALTER TABLE foo DROP COLUMN bar;",
		"ALTER TABLE foo DROP CONSTRAINT c;",
		"TRUNCATE TABLE foo;",
		"DELETE FROM check_against;",
		"DROP VIEW v; DROP SCHEMA s;",
	}
	for _, s := range blocked {
		if !destructiveRE.MatchString(s) {
			t.Errorf("destructiveRE SHOULD match: %q", s)
		}
	}
}

func TestStripSQLComments(t *testing.T) {
	in := "-- DROP TABLE danger\n" +
		"CREATE TABLE x();\n" +
		"/* DROP INDEX evil */ ALTER TABLE x ADD COLUMN c INT;\n" +
		"SELECT 1; -- TRUNCATE comment\n"
	got := stripSQLComments(in)
	want := "\nCREATE TABLE x();\n ALTER TABLE x ADD COLUMN c INT;\nSELECT 1; \n\n"
	if got != want {
		t.Fatalf("stripSQLComments got %q want %q", got, want)
	}
}

func TestGuardDestructive(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "0002_add_col.sql")
	if err := os.WriteFile(good, []byte("-- +goose Up\nALTER TABLE foo ADD COLUMN x TEXT;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "0003_drop.sql")
	if err := os.WriteFile(bad, []byte("-- +goose Up\nDROP TABLE foo;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commentOnly := filepath.Join(dir, "0004_comment.sql")
	if err := os.WriteFile(commentOnly, []byte("-- +goose Up\n-- DROP TABLE but only in a comment\nCREATE TABLE bar (id INT);\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := GuardDestructive(os.DirFS(dir), []string{"0002_add_col.sql"})
	if err != nil || path != "" {
		t.Fatalf("good migration flagged: path=%q err=%v", path, err)
	}

	path, err = GuardDestructive(os.DirFS(dir), []string{"0003_drop.sql"})
	if err == nil {
		t.Fatal("destructive migration should be flagged")
	}
	if path != "0003_drop.sql" {
		t.Fatalf("got path %q, want 0003_drop.sql", path)
	}

	path, err = GuardDestructive(os.DirFS(dir), []string{"0004_comment.sql"})
	if err != nil || path != "" {
		t.Fatalf("comment-only migration flagged: path=%q err=%v", path, err)
	}
}
