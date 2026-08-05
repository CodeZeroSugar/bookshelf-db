package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationStatus describes the state of a single migration.
type MigrationStatus struct {
	Version   int64
	File      string
	AppliedAt *time.Time
	Pending   bool
}

// Migrator applies embedded SQL migrations with goose, tracking applied
// versions in the goose_db_version table so upgrades never re-run or clobber
// existing data.
type Migrator struct {
	provider *goose.Provider
	sqlDB    *sql.DB
	sub      fs.FS
}

func NewMigrator(ctx context.Context, pool *pgxpool.Pool) (*Migrator, error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("open migrations dir: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub)
	if err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("new goose provider: %w", err)
	}
	return &Migrator{provider: provider, sqlDB: sqlDB, sub: sub}, nil
}

func (m *Migrator) Close() { m.sqlDB.Close() }

// Up applies all pending migrations in order. Unless force is set, it refuses
// to apply a pending migration containing destructive statements (the
// additive-only guard). Returns the versions that were applied.
func (m *Migrator) Up(ctx context.Context, force bool) ([]int64, error) {
	pending, err := m.Pending(ctx)
	if err != nil {
		return nil, err
	}
	if !force {
		if err := m.guard(pending); err != nil {
			return nil, err
		}
	}
	results, err := m.provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate up: %w", err)
	}
	versions := make([]int64, 0, len(results))
	for _, r := range results {
		versions = append(versions, r.Source.Version)
	}
	return versions, nil
}

// Status lists all migrations with their state.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate status: %w", err)
	}
	out := make([]MigrationStatus, 0, len(statuses))
	for _, s := range statuses {
		st := MigrationStatus{
			Version: s.Source.Version,
			File:    s.Source.Path,
			Pending: s.State == goose.StatePending,
		}
		if !s.AppliedAt.IsZero() {
			t := s.AppliedAt
			st.AppliedAt = &t
		}
		out = append(out, st)
	}
	return out, nil
}

// Pending returns the sources that have not been applied yet.
func (m *Migrator) Pending(ctx context.Context) ([]*goose.Source, error) {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate status: %w", err)
	}
	var pending []*goose.Source
	for _, s := range statuses {
		if s.State == goose.StatePending {
			pending = append(pending, s.Source)
		}
	}
	return pending, nil
}

// HasPending reports whether any migration is pending.
func (m *Migrator) HasPending(ctx context.Context) (bool, error) {
	p, err := m.Pending(ctx)
	if err != nil {
		return false, err
	}
	return len(p) > 0, nil
}

// destructiveRE matches statements that could destroy data. Used to enforce
// the additive-only migration policy.
var destructiveRE = regexp.MustCompile(
	`(?i)\bDROP\s+(TABLE|INDEX|COLUMN|VIEW|SCHEMA)\b` +
		`|\bALTER\s+TABLE\b[^;]*\bDROP\b` +
		`|\bTRUNCATE\b|\bDELETE\s+FROM\b`)

// guard blocks destructive statements in the Up sections of pending
// migrations.
func (m *Migrator) guard(pending []*goose.Source) error {
	for _, s := range pending {
		content, err := fs.ReadFile(m.sub, s.Path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", s.Path, err)
		}
		body := stripSQLComments(upSection(string(content)))
		if destructiveRE.MatchString(body) {
			return fmt.Errorf(
				"refusing migration %s: contains destructive statement(s) (DROP/TRUNCATE/DELETE). "+
					"Additive-only policy; re-run with '--force' to override", s.Path)
		}
	}
	return nil
}

// GuardDestructive is the same policy check applied to migration files,
// used by tests.
func GuardDestructive(fsys fs.FS, paths []string) (string, error) {
	for _, p := range paths {
		content, err := fs.ReadFile(fsys, p)
		if err != nil {
			return "", err
		}
		body := stripSQLComments(upSection(string(content)))
		if destructiveRE.MatchString(body) {
			return p, fmt.Errorf("migration %s contains destructive statement(s)", p)
		}
	}
	return "", nil
}

// upSection extracts the "-- +goose Up" body of a migration file, stopping at
// the Down marker so rollback statements are never scanned.
func upSection(content string) string {
	if i := strings.Index(content, "-- +goose Up"); i >= 0 {
		content = content[i:]
		if j := strings.Index(content, "-- +goose Down"); j >= 0 {
			content = content[:j]
		}
		return content
	}
	return content
}

// stripSQLComments removes SQL comments so they cannot mask destructive
// statements in the guard scan.
func stripSQLComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			s = s[:i]
			break
		}
		s = s[:i] + s[i+2+j+2:]
	}
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if k := strings.Index(line, "--"); k >= 0 {
			line = line[:k]
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
