package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bookshelf-db/internal/match"
	"bookshelf-db/internal/models"
)

type Store struct {
	pool *pgxpool.Pool
}

type AddResult struct {
	Added   int
	Skipped int
	Hits    []Match
}

type Match struct {
	LibraryTitle  string
	LibraryAuthor string
	CheckTitle    string
	CheckAuthor   string
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// AddCheckAgainst inserts books into check_against, skipping duplicates.
func (s *Store) AddCheckAgainst(ctx context.Context, books []models.Book) (AddResult, error) {
	var res AddResult
	for _, b := range books {
		norm := match.Normalize(b.Title)
		if norm == "" {
			continue
		}
		tag, err := s.pool.Exec(ctx,
			`INSERT INTO check_against (title, author, normalized_title)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (normalized_title) DO NOTHING`,
			b.Title, b.Author, norm)
		if err != nil {
			return res, fmt.Errorf("insert check_against: %w", err)
		}
		if tag.RowsAffected() == 0 {
			res.Skipped++
		} else {
			res.Added++
		}
	}
	return res, nil
}

// AddLibrary inserts books into user_library, skipping duplicates, and
// immediately reports any that exist in check_against.
func (s *Store) AddLibrary(ctx context.Context, books []models.Book) (AddResult, error) {
	var res AddResult
	for _, b := range books {
		norm := match.Normalize(b.Title)
		if norm == "" {
			continue
		}
		tag, err := s.pool.Exec(ctx,
			`INSERT INTO user_library (title, author, normalized_title)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (normalized_title) DO NOTHING`,
			b.Title, b.Author, norm)
		if err != nil {
			return res, fmt.Errorf("insert user_library: %w", err)
		}
		if tag.RowsAffected() == 0 {
			res.Skipped++
			continue
		}
		res.Added++
		var checkID int
		var checkTitle, checkAuthor string
		err = s.pool.QueryRow(ctx,
			`SELECT id, title, author FROM check_against WHERE normalized_title = $1`, norm).
			Scan(&checkID, &checkTitle, &checkAuthor)
		if err == nil {
			res.Hits = append(res.Hits, Match{
				LibraryTitle: b.Title, LibraryAuthor: b.Author,
				CheckTitle: checkTitle, CheckAuthor: checkAuthor,
			})
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return res, fmt.Errorf("check match: %w", err)
		}
	}
	return res, nil
}

// RemoveCheckAgainst deletes an exact title match. If none exists, returns
// close matches (best to worst) for the caller to offer as choices.
func (s *Store) RemoveCheckAgainst(ctx context.Context, title string) (bool, []models.Book, error) {
	return s.remove(ctx, "check_against", title)
}

// RemoveLibrary deletes an exact title match. If none exists, returns close
// matches (best to worst) for the caller to offer as choices.
func (s *Store) RemoveLibrary(ctx context.Context, title string) (bool, []models.Book, error) {
	return s.remove(ctx, "user_library", title)
}

func (s *Store) remove(ctx context.Context, table, title string) (bool, []models.Book, error) {
	norm := match.Normalize(title)
	tag, err := s.pool.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE normalized_title = $1`, table), norm)
	if err != nil {
		return false, nil, fmt.Errorf("delete %s: %w", table, err)
	}
	if tag.RowsAffected() > 0 {
		return true, nil, nil
	}
	suggestions, err := s.closeMatches(ctx, table, norm, 5)
	if err != nil {
		return false, nil, err
	}
	return false, suggestions, nil
}

func (s *Store) closeMatches(ctx context.Context, table, norm string, limit int) ([]models.Book, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT title, author, similarity(normalized_title, $1) AS sim
		             FROM %s
		             WHERE normalized_title <> $1 AND similarity(normalized_title, $1) >= 0.3
		             ORDER BY sim DESC, title
		             LIMIT $2`, table), norm, limit)
	if err != nil {
		return nil, fmt.Errorf("similarity query: %w", err)
	}
	defer rows.Close()
	var books []models.Book
	for rows.Next() {
		var b models.Book
		var sim float64
		if err := rows.Scan(&b.Title, &b.Author, &sim); err != nil {
			return nil, fmt.Errorf("scan similar: %w", err)
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// Lookup finds an exact normalized-title match in table ("check_against" or
// "user_library"). If none exists, returns close matches for the caller to
// show as suggestions.
func (s *Store) Lookup(ctx context.Context, table, title string) (models.Book, bool, []models.Book, error) {
	norm := match.Normalize(title)
	var book models.Book
	err := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT title, author FROM %s WHERE normalized_title = $1`, table), norm).
		Scan(&book.Title, &book.Author)
	if err == nil {
		return book, true, nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return models.Book{}, false, nil, fmt.Errorf("lookup %s: %w", table, err)
	}
	suggestions, err := s.closeMatches(ctx, table, norm, 5)
	if err != nil {
		return models.Book{}, false, nil, err
	}
	return models.Book{}, false, suggestions, nil
}

// AllCheckAgainst returns every row in check_against, ordered by title.
func (s *Store) AllCheckAgainst(ctx context.Context) ([]models.Book, error) {
	return s.all(ctx, "check_against")
}

// AllLibrary returns every row in user_library, ordered by title.
func (s *Store) AllLibrary(ctx context.Context) ([]models.Book, error) {
	return s.all(ctx, "user_library")
}

func (s *Store) all(ctx context.Context, table string) ([]models.Book, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT title, author FROM %s ORDER BY title`, table))
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", table, err)
	}
	defer rows.Close()
	var books []models.Book
	for rows.Next() {
		var b models.Book
		if err := rows.Scan(&b.Title, &b.Author); err != nil {
			return nil, fmt.Errorf("scan %s: %w", table, err)
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// Missing returns books on the check-against list that the user does not own.
func (s *Store) Missing(ctx context.Context) ([]models.Book, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.title, c.author
		 FROM check_against c
		 LEFT JOIN user_library l ON l.normalized_title = c.normalized_title
		 WHERE l.id IS NULL
		 ORDER BY c.title`)
	if err != nil {
		return nil, fmt.Errorf("missing query: %w", err)
	}
	defer rows.Close()
	var books []models.Book
	for rows.Next() {
		var b models.Book
		if err := rows.Scan(&b.Title, &b.Author); err != nil {
			return nil, fmt.Errorf("scan missing: %w", err)
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

func (s *Store) Compare(ctx context.Context) ([]Match, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT l.id, c.id
		 FROM user_library l
		 JOIN check_against c ON l.normalized_title = c.normalized_title`)
	if err != nil {
		return nil, fmt.Errorf("compare query: %w", err)
	}
	var pairs [][2]int
	for rows.Next() {
		var l, c int
		if err := rows.Scan(&l, &c); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan pair: %w", err)
		}
		pairs = append(pairs, [2]int{l, c})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compare rows: %w", err)
	}

	for _, p := range pairs {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO matches (library_book_id, check_book_id)
			 VALUES ($1, $2)
			 ON CONFLICT (library_book_id, check_book_id) DO NOTHING`,
			p[0], p[1]); err != nil {
			return nil, fmt.Errorf("upsert match: %w", err)
		}
	}

	// Drop matches that no longer hold (title edited/re-normalized upstream).
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM matches m
		 USING user_library l, check_against c
		 WHERE m.library_book_id = l.id AND m.check_book_id = c.id
		   AND l.normalized_title <> c.normalized_title`); err != nil {
		return nil, fmt.Errorf("prune matches: %w", err)
	}

	return s.Matches(ctx)
}

// Matches lists the current overlap rows.
func (s *Store) Matches(ctx context.Context) ([]Match, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT l.title, l.author, c.title, c.author
		 FROM matches m
		 JOIN user_library l ON l.id = m.library_book_id
		 JOIN check_against c ON c.id = m.check_book_id
		 ORDER BY l.title`)
	if err != nil {
		return nil, fmt.Errorf("matches query: %w", err)
	}
	defer rows.Close()
	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.LibraryTitle, &m.LibraryAuthor, &m.CheckTitle, &m.CheckAuthor); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Count returns row counts keyed by table.
func (s *Store) Count(ctx context.Context) (check, library, matches int, err error) {
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM check_against`).Scan(&check); err != nil {
		return
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM user_library`).Scan(&library); err != nil {
		return
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM matches`).Scan(&matches); err != nil {
		return
	}
	return
}

// Clear removes all rows from the requested scope: "check", "library", or "all".
func (s *Store) Clear(ctx context.Context, scope string) (removed int, err error) {
	switch scope {
	case "check":
		tag, e := s.pool.Exec(ctx, `DELETE FROM check_against`)
		err, removed = e, int(tag.RowsAffected())
	case "library":
		tag, e := s.pool.Exec(ctx, `DELETE FROM user_library`)
		err, removed = e, int(tag.RowsAffected())
	case "all":
		var c, l int
		tag, e := s.pool.Exec(ctx, `DELETE FROM check_against`)
		if e != nil {
			err = e
			return
		}
		c = int(tag.RowsAffected())
		tag, e = s.pool.Exec(ctx, `DELETE FROM user_library`)
		if e != nil {
			err = e
			return
		}
		l = int(tag.RowsAffected())
		removed, err = c+l, nil
	default:
		err = fmt.Errorf("unknown scope %q (want check, library, or all)", scope)
	}
	return
}
