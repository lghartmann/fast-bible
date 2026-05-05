package main

import (
	"log"

	"github.com/lghartmann/fast-bible/app/elasticsearch"
	"github.com/lghartmann/fast-bible/app/extract"
)

var BIBLE string = "bible"

func main() {
	data := extract.ExtractFromJSON()
	es := elasticsearch.NewElasticSearch()

	err := elasticsearch.CreateMapping(BIBLE, elasticsearch.BuildVerseMapping(), es)
	if err != nil {
		log.Fatalf("err creating mapping: %s", err)
	}

	err = elasticsearch.IndexVerses(BIBLE, data, es)
	if err != nil {
		log.Fatalf("err indexing verses: %s", err)
	}
}
