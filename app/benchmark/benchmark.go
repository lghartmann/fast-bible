package benchmark

import (
	"fmt"
	"strings"
	"time"

	"github.com/lghartmann/fast-bible/app/extract"
)

const SampleLimit = 3

type Result struct {
	Source   string
	Query    string
	Hits     int
	Duration time.Duration
	Samples  []extract.VerseDocument
}

func (r Result) SummaryLine() string {
	return fmt.Sprintf(
		"source=%s | query=%q | hits=%d | duration=%s",
		r.Source,
		r.Query,
		r.Hits,
		r.Duration.Round(time.Microsecond),
	)
}

func FormatSample(doc extract.VerseDocument) string {
	return fmt.Sprintf(
		"%s %d:%d | %s",
		doc.Book,
		doc.Chapter,
		doc.Number,
		truncate(doc.Verse, 110),
	)
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}

	return strings.TrimSpace(s[:limit-3]) + "..."
}
