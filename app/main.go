package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
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

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}
	elasticAddr := os.Getenv("ELASTIC_SEARCH_UNSAFE_ADDRESS")

	data := extract.ExtractFromJSON()
	log.Printf("loaded %d verses from JSON", len(data))

	if err := populateElasticSearch(elasticAddr, data); err != nil {
		log.Fatalf("err populating elasticsearch: %s", err)
	}

	if err := populatePostgresDatabases(data); err != nil {
		log.Fatalf("err populating postgres: %s", err)
	}

	if err := populateMySQLDatabases(data); err != nil {
		log.Fatalf("err populating mysql: %s", err)
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

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
