package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/joho/godotenv"
	"github.com/lghartmann/fast-bible/app/benchmark"
	es "github.com/lghartmann/fast-bible/app/es"
	"github.com/lghartmann/fast-bible/app/extract"
	"github.com/lghartmann/fast-bible/app/mysql"
	"github.com/lghartmann/fast-bible/app/postgres"
)

const (
	bibleIndex        = "bible_v2"
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

var pgConfig postgres.Config
var pgDB *sql.DB
var pgFTSConfig postgres.Config
var pgFTSDB *sql.DB
var mysqlConfig mysql.Config
var myDB *sql.DB
var mysqlFTSConfig mysql.Config
var myFTSDB *sql.DB
var esClient *elasticsearch.TypedClient

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := initClients(); err != nil {
		log.Fatal(err)
	}
	defer closeClients()

	data := extract.ExtractFromJSON()
	log.Printf("loaded %d verses from JSON", len(data))

	hasDocAtEs, err := es.CheckESDataExistence(bibleIndex, esClient)
	if err != nil {
		log.Fatal(err)
	}

	hasDocAtPG, err := postgresDatabasesHaveData()
	if err != nil {
		log.Fatal(err)
	}

	hasDocAtMySQL, err := mysqlDatabasesHaveData()
	if err != nil {
		log.Fatal(err)
	}

	if !hasDocAtEs {
		if err := populateElasticSearch(data); err != nil {
			log.Fatal(err)
		}
	}

	if !hasDocAtPG {
		if err := populatePostgresDatabases(data); err != nil {
			log.Fatal(err)
		}
	}

	if !hasDocAtMySQL {
		if err := populateMySQLDatabases(data); err != nil {
			log.Fatal(err)
		}
	}

	if err := runBenchmarks(); err != nil {
		log.Fatalf("err running benchmarks: %s", err)
	}
}

func initClients() error {
	pgConfig = postgres.Config{
		Address:  os.Getenv("POSTGRES_URL"),
		Database: getenvDefault("POSTGRES_DB", defaultPostgresDB),
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
	}
	var err error
	pgDB, err = postgres.Open(pgConfig)
	if err != nil {
		return err
	}

	pgFTSConfig = pgConfig
	pgFTSConfig.Database = getenvDefault("POSTGRES_FTS_DB", defaultFTSDB)
	pgFTSDB, err = postgres.Open(pgFTSConfig)
	if err != nil {
		_ = pgDB.Close()
		return err
	}

	mysqlConfig = mysql.Config{
		Address:  os.Getenv("MYSQL_URL"),
		Database: getenvDefault("MYSQL_DB", defaultMySQLDB),
		User:     os.Getenv("MYSQL_USER"),
		Password: os.Getenv("MYSQL_PASSWORD"),
	}
	myDB, err = mysql.Open(mysqlConfig)
	if err != nil {
		_ = pgFTSDB.Close()
		_ = pgDB.Close()
		return err
	}

	mysqlFTSConfig = mysqlConfig
	mysqlFTSConfig.Database = getenvDefault("MYSQL_FTS_DB", defaultFTSDB)
	myFTSDB, err = mysql.Open(mysqlFTSConfig)
	if err != nil {
		_ = myDB.Close()
		_ = pgFTSDB.Close()
		_ = pgDB.Close()
		return err
	}

	elasticAddr := os.Getenv("ELASTIC_SEARCH_UNSAFE_ADDRESS")
	esClient = es.NewElasticSearch(elasticAddr)

	return nil
}

func closeClients() {
	if myFTSDB != nil {
		_ = myFTSDB.Close()
	}
	if myDB != nil {
		_ = myDB.Close()
	}
	if pgFTSDB != nil {
		_ = pgFTSDB.Close()
	}
	if pgDB != nil {
		_ = pgDB.Close()
	}
}

func populateElasticSearch(data []extract.VerseDocument) error {
	log.Printf("populating Elasticsearch index %q", bibleIndex)

	if err := es.CreateMapping(bibleIndex, es.BuildVerseMapping(), esClient); err != nil {
		return err
	}

	return es.IndexVerses(bibleIndex, data, esClient)
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

func postgresDatabasesHaveData() (bool, error) {
	if err := postgres.EnsureSchema(pgDB, false); err != nil {
		return false, err
	}
	if err := postgres.EnsureSchema(pgFTSDB, true); err != nil {
		return false, err
	}

	hasPrimary, err := postgres.CheckPGDataExistence(pgDB)
	if err != nil {
		return false, err
	}

	hasFTS, err := postgres.CheckPGDataExistence(pgFTSDB)
	if err != nil {
		return false, err
	}

	return hasPrimary && hasFTS, nil
}

func mysqlDatabasesHaveData() (bool, error) {
	if err := mysql.EnsureSchema(myDB, false); err != nil {
		return false, err
	}
	if err := mysql.EnsureSchema(myFTSDB, true); err != nil {
		return false, err
	}

	hasPrimary, err := mysql.CheckMySQLDataExistence(myDB)
	if err != nil {
		return false, err
	}

	hasFTS, err := mysql.CheckMySQLDataExistence(myFTSDB)
	if err != nil {
		return false, err
	}

	return hasPrimary && hasFTS, nil
}

func runBenchmarks() error {
	log.Printf("benchmark starting with %d queries across 5 data sources", len(benchmarkQueries))

	runners := []struct {
		run func(string) (benchmark.Result, error)
	}{
		{
			run: func(query string) (benchmark.Result, error) {
				return benchmarkSearch("elasticsearch", query, func() (int, []extract.VerseDocument, error) {
					return es.SearchVerses(bibleIndex, query, benchmark.SampleLimit, esClient)
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

	log.Printf("warmup pass (results discarded)")
	for _, query := range benchmarkQueries {
		for _, runner := range runners {
			if _, err := runner.run(query); err != nil {
				return err
			}
		}
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
