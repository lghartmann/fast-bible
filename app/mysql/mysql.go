package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/lghartmann/fast-bible/app/extract"
)

type Config struct {
	Address  string
	Database string
	User     string
	Password string
}

func Open(cfg Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s)/%s?charset=utf8mb4&collation=utf8mb4_unicode_ci&parseTime=true",
		cfg.User,
		cfg.Password,
		cfg.Address,
		cfg.Database,
	)

	db, err := sql.Open("mysql", dsn)
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
			book VARCHAR(100) NOT NULL,
			chapter INT NOT NULL,
			number INT NOT NULL,
			verse TEXT NOT NULL,
			PRIMARY KEY (book, chapter, number),
			INDEX idx_verses_book (book),
			INDEX idx_verses_book_chapter (book, chapter)
		) ENGINE=InnoDB`,
	}

	if fullText {
		statements = append(statements,
			`CREATE FULLTEXT INDEX idx_verses_verse ON verses (verse)`,
		)
	} else {
		statements = append(statements,
			`CREATE INDEX idx_verses_chapter_number ON verses (chapter, number)`,
		)
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateIndexError(err) {
			return err
		}
	}

	return nil
}

func SearchVerses(db *sql.DB, query string, limit int) (int, []extract.VerseDocument, error) {
	total, err := countVerses(db, `
		SELECT COUNT(*)
		FROM verses
		WHERE verse LIKE CONCAT('%', ?, '%')
	`, query)
	if err != nil {
		return 0, nil, err
	}

	rows, err := db.Query(`
		SELECT book, chapter, number, verse
		FROM verses
		WHERE verse LIKE CONCAT('%', ?, '%')
		ORDER BY book, chapter, number
		LIMIT ?
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
	exactPhrase := `"` + query + `"`
	total, err := countVerses(db, `
		SELECT COUNT(*)
		FROM verses
		WHERE MATCH(verse) AGAINST(? IN BOOLEAN MODE)
	`, exactPhrase)
	if err != nil {
		return 0, nil, err
	}

	rows, err := db.Query(`
		SELECT book, chapter, number, verse
		FROM verses
		WHERE MATCH(verse) AGAINST(? IN BOOLEAN MODE)
		ORDER BY book, chapter, number
		LIMIT ?
	`, exactPhrase, limit)
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
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE verse = VALUES(verse)
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

func CheckMySQLDataExistence(db *sql.DB) (bool, error) {
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM verses`).Scan(&total); err != nil {
		return false, err
	}

	return total > 0, nil
}

func isDuplicateIndexError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Duplicate key name") || strings.Contains(err.Error(), "already exists"))
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
