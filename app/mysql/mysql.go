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

func isDuplicateIndexError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "Duplicate key name") || strings.Contains(err.Error(), "already exists"))
}
