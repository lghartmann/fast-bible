package main

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/lghartmann/fast-bible/app/benchmark"
	"github.com/lghartmann/fast-bible/app/elasticsearch"
	"github.com/lghartmann/fast-bible/app/extract"
	"github.com/lghartmann/fast-bible/app/mysql"
	"github.com/lghartmann/fast-bible/app/postgres"
)

const (
	bibleIndex        = "bible"
	defaultPostgresDB = "bible"
	defaultMySQLDB    = "bible"
	defaultFTSDB      = "bible_fts"
)

var benchmarkQueries = []string{
	"a verdade e a vida",
	"tribulações",
	"perdoa",
	"nos amou",
	"o senhor é",
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
	elasticAddr := os.Getenv("ELASTIC_SEARCH_UNSAFE_ADDRESS")

	data := extract.ExtractFromJSON()
	log.Printf("loaded %d verses from JSON", len(data))

	if err := runBenchmarks(elasticAddr); err != nil {
		log.Fatalf("err running benchmarks: %s", err)
	}
}

func populateElasticSearch(addr string, data []extract.VerseDocument) error {
	log.Printf("populating Elasticsearch index %q", bibleIndex)

	es := elasticsearch.NewElasticSearch(addr)

	if err := elasticsearch.CreateMapping(bibleIndex, elasticsearch.BuildVerseMapping(), es); err != nil {
		return err
	}

	return elasticsearch.IndexVerses(bibleIndex, data, es)
}

func populatePostgresDatabases(data []extract.VerseDocument) error {
	baseConfig := postgres.Config{
		Address:  os.Getenv("POSTGRES_URL"),
		Database: getenvDefault("POSTGRES_DB", defaultPostgresDB),
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
	}

	if err := populatePostgresDatabase("postgres", baseConfig, false, data); err != nil {
		return err
	}

	ftsConfig := baseConfig
	ftsConfig.Database = getenvDefault("POSTGRES_FTS_DB", defaultFTSDB)
	return populatePostgresDatabase("postgres_fts", ftsConfig, true, data)
}

func populatePostgresDatabase(name string, cfg postgres.Config, fullText bool, data []extract.VerseDocument) error {
	log.Printf("populating %s database %q", name, cfg.Database)

	db, err := postgres.Open(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := postgres.EnsureSchema(db, fullText); err != nil {
		return err
	}

	return postgres.InsertVerses(db, data)
}

func populateMySQLDatabases(data []extract.VerseDocument) error {
	baseConfig := mysql.Config{
		Address:  os.Getenv("MYSQL_URL"),
		Database: getenvDefault("MYSQL_DB", defaultMySQLDB),
		User:     os.Getenv("MYSQL_USER"),
		Password: os.Getenv("MYSQL_PASSWORD"),
	}

	if err := populateMySQLDatabase("mysql", baseConfig, false, data); err != nil {
		return err
	}

	ftsConfig := baseConfig
	ftsConfig.Database = getenvDefault("MYSQL_FTS_DB", defaultFTSDB)
	return populateMySQLDatabase("mysql_fts", ftsConfig, true, data)
}

func populateMySQLDatabase(name string, cfg mysql.Config, fullText bool, data []extract.VerseDocument) error {
	log.Printf("populating %s database %q", name, cfg.Database)

	db, err := mysql.Open(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := mysql.EnsureSchema(db, fullText); err != nil {
		return err
	}

	return mysql.InsertVerses(db, data)
}

func runBenchmarks(elasticAddr string) error {
	log.Printf("benchmark starting with %d queries across 5 data sources", len(benchmarkQueries))

	es := elasticsearch.NewElasticSearch(elasticAddr)

	pgConfig := postgres.Config{
		Address:  os.Getenv("POSTGRES_URL"),
		Database: getenvDefault("POSTGRES_DB", defaultPostgresDB),
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
	}
	pgDB, err := postgres.Open(pgConfig)
	if err != nil {
		return err
	}
	defer pgDB.Close()

	pgFTSConfig := pgConfig
	pgFTSConfig.Database = getenvDefault("POSTGRES_FTS_DB", defaultFTSDB)
	pgFTSDB, err := postgres.Open(pgFTSConfig)
	if err != nil {
		return err
	}
	defer pgFTSDB.Close()

	mysqlConfig := mysql.Config{
		Address:  os.Getenv("MYSQL_URL"),
		Database: getenvDefault("MYSQL_DB", defaultMySQLDB),
		User:     os.Getenv("MYSQL_USER"),
		Password: os.Getenv("MYSQL_PASSWORD"),
	}
	myDB, err := mysql.Open(mysqlConfig)
	if err != nil {
		return err
	}
	defer myDB.Close()

	mysqlFTSConfig := mysqlConfig
	mysqlFTSConfig.Database = getenvDefault("MYSQL_FTS_DB", defaultFTSDB)
	myFTSDB, err := mysql.Open(mysqlFTSConfig)
	if err != nil {
		return err
	}
	defer myFTSDB.Close()

	runners := []struct {
		run func(string) (benchmark.Result, error)
	}{
		{
			run: func(query string) (benchmark.Result, error) {
				return benchmarkSearch("elasticsearch", query, func() (int, []extract.VerseDocument, error) {
					return elasticsearch.SearchVerses(bibleIndex, query, benchmark.SampleLimit, es)
				})
			},
		},
		{
			run: func(query string) (benchmark.Result, error) {
				return benchmarkSearch("postgres", query, func() (int, []extract.VerseDocument, error) {
					return postgres.SearchVerses(pgDB, query, benchmark.SampleLimit)
				})
			},
		},
		{
			run: func(query string) (benchmark.Result, error) {
				return benchmarkSearch("postgres_fts", query, func() (int, []extract.VerseDocument, error) {
					return postgres.SearchVersesFTS(pgFTSDB, query, benchmark.SampleLimit)
				})
			},
		},
		{
			run: func(query string) (benchmark.Result, error) {
				return benchmarkSearch("mysql", query, func() (int, []extract.VerseDocument, error) {
					return mysql.SearchVerses(myDB, query, benchmark.SampleLimit)
				})
			},
		},
		{
			run: func(query string) (benchmark.Result, error) {
				return benchmarkSearch("mysql_fts", query, func() (int, []extract.VerseDocument, error) {
					return mysql.SearchVersesFTS(myFTSDB, query, benchmark.SampleLimit)
				})
			},
		},
	}

	overallStarted := time.Now()
	for _, query := range benchmarkQueries {
		log.Printf("------------------------------------------------------------")
		log.Printf("query=%q", query)

		for _, runner := range runners {
			result, err := runner.run(query)
			if err != nil {
				return err
			}

			log.Printf("%s", result.SummaryLine())
			for _, sample := range result.Samples {
				log.Printf("sample | %s", benchmark.FormatSample(sample))
			}
			if len(result.Samples) == 0 {
				log.Printf("sample | no hits")
			}
		}
	}

	log.Printf("------------------------------------------------------------")
	log.Printf("benchmark completed in %s", time.Since(overallStarted).Round(time.Microsecond))

	return nil
}

func benchmarkSearch(source string, query string, search func() (int, []extract.VerseDocument, error)) (benchmark.Result, error) {
	started := time.Now()
	total, samples, err := search()
	if err != nil {
		return benchmark.Result{}, err
	}

	return benchmark.Result{
		Source:   source,
		Query:    query,
		Hits:     total,
		Duration: time.Since(started),
		Samples:  samples,
	}, nil
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
