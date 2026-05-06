package postgres

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lghartmann/fast-bible/app/extract"
)

type Config struct {
	Address  string
	Database string
	User     string
	Password string
}

func Open(cfg Config) (*sql.DB, error) {
	dsn := buildDSN(cfg)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func EnsureSchema(db *sql.DB, fullText bool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS verses (
			book TEXT NOT NULL,
			chapter INTEGER NOT NULL,
			number INTEGER NOT NULL,
			verse TEXT NOT NULL,
			PRIMARY KEY (book, chapter, number)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_verses_book ON verses (book)`,
		`CREATE INDEX IF NOT EXISTS idx_verses_book_chapter ON verses (book, chapter)`,
	}

	if fullText {
		statements = append(statements,
			`ALTER TABLE verses
				ADD COLUMN IF NOT EXISTS verse_tsv tsvector
				GENERATED ALWAYS AS (to_tsvector('simple', verse)) STORED`,
			`CREATE INDEX IF NOT EXISTS idx_verses_verse_tsv ON verses USING GIN (verse_tsv)`,
		)
	} else {
		statements = append(statements,
			`CREATE INDEX IF NOT EXISTS idx_verses_chapter_number ON verses (chapter, number)`,
		)
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func SearchVerses(db *sql.DB, query string, limit int) (int, []extract.VerseDocument, error) {
	total, err := countVerses(db, `
		SELECT COUNT(*)
		FROM verses
		WHERE verse ILIKE '%' || $1 || '%'
	`, query)
	if err != nil {
		return 0, nil, err
	}

	rows, err := db.Query(`
		SELECT book, chapter, number, verse
		FROM verses
		WHERE verse ILIKE '%' || $1 || '%'
		ORDER BY book, chapter, number
		LIMIT $2
	`, query, limit)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	verses, err := scanVerses(rows)
	if err != nil {
		return 0, nil, err
	}

	return total, verses, nil
}

func SearchVersesFTS(db *sql.DB, query string, limit int) (int, []extract.VerseDocument, error) {
	total, err := countVerses(db, `
		SELECT COUNT(*)
		FROM verses
		WHERE verse_tsv @@ phraseto_tsquery('simple', $1)
	`, query)
	if err != nil {
		return 0, nil, err
	}

	rows, err := db.Query(`
		SELECT book, chapter, number, verse
		FROM verses
		WHERE verse_tsv @@ phraseto_tsquery('simple', $1)
		ORDER BY book, chapter, number
		LIMIT $2
	`, query, limit)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	verses, err := scanVerses(rows)
	if err != nil {
		return 0, nil, err
	}

	return total, verses, nil
}

func InsertVerses(db *sql.DB, verses []extract.VerseDocument) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO verses (book, chapter, number, verse)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (book, chapter, number)
		DO UPDATE SET verse = EXCLUDED.verse
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, verse := range verses {
		if _, err := stmt.Exec(verse.Book, verse.Chapter, verse.Number, verse.Verse); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("inserting %s %d:%d: %w", verse.Book, verse.Chapter, verse.Number, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func buildDSN(cfg Config) string {
	if strings.Contains(cfg.Address, "://") {
		parsed, err := url.Parse(cfg.Address)
		if err != nil {
			return ""
		}

		parsed.User = url.UserPassword(cfg.User, cfg.Password)
		parsed.Path = "/" + cfg.Database
		query := parsed.Query()
		if query.Get("sslmode") == "" {
			query.Set("sslmode", "disable")
		}
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}

	parsed := &url.URL{
		Scheme: "postgres",
		Host:   cfg.Address,
		Path:   "/" + cfg.Database,
		User:   url.UserPassword(cfg.User, cfg.Password),
	}

	query := parsed.Query()
	query.Set("sslmode", "disable")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func scanVerses(rows *sql.Rows) ([]extract.VerseDocument, error) {
	verses := make([]extract.VerseDocument, 0)
	for rows.Next() {
		var verse extract.VerseDocument
		if err := rows.Scan(&verse.Book, &verse.Chapter, &verse.Number, &verse.Verse); err != nil {
			return nil, err
		}
		verses = append(verses, verse)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return verses, nil
}

func countVerses(db *sql.DB, query string, search string) (int, error) {
	var total int
	if err := db.QueryRow(query, search).Scan(&total); err != nil {
		return 0, err
	}

	return total, nil
}
