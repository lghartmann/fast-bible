package extract

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"strings"
)

type BibleFromJson struct {
	Abbrev   string     `json:"abbrev"`
	Chapters [][]string `json:"chapters"`
	Name     string     `json:"name"`
}

type VerseDocument struct {
	Number  int
	Chapter int
	Book    string
	Verse   string
}

func ExtractFromJSON() []VerseDocument {
	f, err := os.ReadFile("app/aa.json")
	if err != nil {
		log.Fatalf("err reading file: %s", err)
	}
	f = bytes.TrimPrefix(f, []byte("\xef\xbb\xbf"))

	bible := []BibleFromJson{}

	err = json.Unmarshal(f, &bible)
	if err != nil {
		log.Fatalf("err unmarshaling file: %s", err)
	}

	acc := make([]VerseDocument, 0)

	for _, book := range bible {
		for chapterIndex, chapter := range book.Chapters {
			for verseIndex, verse := range chapter {
				cleanedVerse := cleanVerse(verse)
				if cleanedVerse == "" {
					continue
				}

				doc := VerseDocument{
					Chapter: chapterIndex + 1,
					Number:  verseIndex + 1,
					Verse:   cleanedVerse,
					Book:    book.Name,
				}
				acc = append(acc, doc)
			}
		}
	}

	return acc
}

func cleanVerse(verse string) string {
	verse = strings.Join(strings.Fields(verse), " ")
	verse = strings.ReplaceAll(verse, " ;", ";")
	verse = strings.ReplaceAll(verse, " ,", ",")
	verse = strings.ReplaceAll(verse, "}", "")
	verse = strings.ReplaceAll(verse, "{", "")
	verse = strings.ReplaceAll(verse, " .", ".")
	return verse
}
