package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// newTestDB creates a throwaway database on the dedicated dev/test postgres
// and returns a pool to it. It skips the test when postgres is not reachable,
// refuses non-localhost targets, and refuses to run against the application's
// production database (DATABASE_URL).
func newTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	testURL := os.Getenv("BOOKSHELF_TEST_URL")
	if testURL == "" {
		testURL = os.Getenv("BOOKSHELF_DEV_URL")
	}
	if testURL == "" {
		testURL = "postgres://bookshelf:bookshelf@localhost:5433/bookshelf_dev"
	}

	cfg, err := pgxpool.ParseConfig(testURL)
	if err != nil {
		t.Skipf("cannot parse test database url, skipping: %v", err)
	}
	host := cfg.ConnConfig.Host
	if !isLoopback(host) {
		t.Skipf("refusing to run integration tests against non-localhost host %q", host)
	}
	if sameTarget(testURL, DefaultURL()) {
		t.Skip("refusing to run integration tests against the application database (DATABASE_URL)")
	}

	admin, err := connectURL(ctx, testURL)
	if err != nil {
		t.Skipf("postgres not available, skipping integration test: %v", err)
	}
	name := fmt.Sprintf("bookshelf_mig_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name)
		admin.Close()
	})
	return pool
}

// sameTarget reports whether two URLs point at the same host, port, and
// database.
func sameTarget(a, b string) bool {
	ca, errA := pgxpool.ParseConfig(a)
	cb, errB := pgxpool.ParseConfig(b)
	if errA != nil || errB != nil {
		return false
	}
	return ca.ConnConfig.Host == cb.ConnConfig.Host &&
		ca.ConnConfig.Port == cb.ConnConfig.Port &&
		ca.ConnConfig.Database == cb.ConnConfig.Database
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func TestSameTarget(t *testing.T) {
	prod := "postgres://bookshelf:bookshelf@localhost:5432/bookshelf"
	dev := "postgres://bookshelf:bookshelf@localhost:5433/bookshelf_dev"
	if !sameTarget(prod, prod) {
		t.Error("same URL should be same target")
	}
	if sameTarget(prod, dev) {
		t.Error("prod and dev should differ")
	}
	if sameTarget(prod, "postgres://bookshelf:bookshelf@localhost:5432/other") {
		t.Error("same host/port but different database should differ")
	}
}

func TestIsLoopback(t *testing.T) {
	for _, ok := range []string{"localhost", "127.0.0.1", "::1"} {
		if !isLoopback(ok) {
			t.Errorf("expected %q to be loopback", ok)
		}
	}
	for _, no := range []string{"db.example.com", "10.0.0.1", ""} {
		if isLoopback(no) {
			t.Errorf("expected %q to NOT be loopback", no)
		}
	}
}

func TestMigrateFreshAndIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := newTestDB(t)

	mig, err := NewMigrator(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer mig.Close()

	pending, err := mig.HasPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("fresh database should report pending migrations")
	}

	versions, err := mig.Up(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0] != 1 {
		t.Fatalf("expected version 1 applied, got %v", versions)
	}

	var table string
	if err := pool.QueryRow(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename IN ('check_against','user_library','matches') ORDER BY tablename`).
		Scan(&table); err != nil {
		t.Fatalf("expected tables to exist: %v", err)
	}

	pending, err = mig.HasPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("no migrations should be pending after up")
	}

	versions, err = mig.Up(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("second up should be a no-op, got %v", versions)
	}
}

func TestMigratePreservesData(t *testing.T) {
	ctx := context.Background()
	pool := newTestDB(t)

	mig, err := NewMigrator(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer mig.Close()
	if _, err := mig.Up(ctx, false); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO check_against (title, author, normalized_title) VALUES ('Keep Me', 'A', 'keep me')`); err != nil {
		t.Fatal(err)
	}

	// Re-running migrations must not touch existing rows.
	if _, err := mig.Up(ctx, false); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM check_against WHERE title='Keep Me'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("row lost across migration re-run: count=%d", n)
	}
}

func TestMigrateGuardBlocksDestructive(t *testing.T) {
	ctx := context.Background()
	pool := newTestDB(t)

	dir := t.TempDir()
	files := map[string]string{
		"0001_good.sql":        "-- +goose Up\nCREATE TABLE guard_a (id INT);\n",
		"0002_destructive.sql": "-- +goose Up\nDROP TABLE guard_a;\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	fsys := os.DirFS(dir)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys)
	if err != nil {
		t.Fatal(err)
	}
	m := &Migrator{provider: provider, sqlDB: sqlDB, sub: fsys}

	if _, err := m.Up(ctx, false); err == nil {
		t.Fatal("expected destructive migration to be blocked")
	}

	versions, err := m.Up(ctx, true)
	if err != nil {
		t.Fatalf("force should allow applying: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions applied with force, got %v", versions)
	}
}
