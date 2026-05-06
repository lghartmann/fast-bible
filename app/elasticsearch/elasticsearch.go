package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/elastic/go-elasticsearch/v9"
	essearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/lghartmann/fast-bible/app/extract"
)

func NewElasticSearch(addr string) *elasticsearch.TypedClient {
	es, err := elasticsearch.NewTyped(
		elasticsearch.WithAddresses(addr),
		elasticsearch.WithRetry(3, http.StatusInternalServerError),
	)
	if err != nil {
		log.Fatalf("err es: %s", err)
	}

	return es
}

func CreateMapping(name string, mapping types.TypeMappingVariant, es *elasticsearch.TypedClient) error {
	exists, err := es.Indices.Exists(name).Do(context.Background())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	res, err := es.Indices.Create(name).Mappings(mapping).Do(context.Background())
	if err != nil {
		return err
	}

	_ = res
	return nil
}

func BuildVerseMapping() *types.TypeMapping {
	mapping := types.NewTypeMapping()
	mapping.Properties = map[string]types.Property{
		"book":    types.NewKeywordProperty(),
		"chapter": types.NewIntegerNumberProperty(),
		"number":  types.NewIntegerNumberProperty(),
		"verse":   types.NewTextProperty(),
	}

	return mapping
}

func IndexVerses(indexName string, verses []extract.VerseDocument, es *elasticsearch.TypedClient) error {
	for _, verse := range verses {
		docID := fmt.Sprintf("%s-%d-%d", verse.Book, verse.Chapter, verse.Number)

		_, err := es.Index(indexName).
			Id(docID).
			Document(verse).
			Do(context.Background())
		if err != nil {
			return fmt.Errorf("indexing %s: %w", docID, err)
		}
	}

	return nil
}

func SearchVerses(indexName string, query string, limit int, es *elasticsearch.TypedClient) (int, []extract.VerseDocument, error) {
	matchPhrase := types.NewMatchPhraseQuery()
	matchPhrase.Query = query

	res, err := es.Search().
		Index(indexName).
		Request(&essearch.Request{
			Query: &types.Query{
				MatchPhrase: map[string]types.MatchPhraseQuery{
					"verse": *matchPhrase,
				},
			},
			Size: &limit,
		}).
		Do(context.Background())
	if err != nil {
		return 0, nil, err
	}

	verses := make([]extract.VerseDocument, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		if len(hit.Source_) == 0 {
			continue
		}

		var verse extract.VerseDocument
		if err := json.Unmarshal(hit.Source_, &verse); err != nil {
			return 0, nil, fmt.Errorf("decoding elasticsearch hit: %w", err)
		}

		verses = append(verses, verse)
	}

	total := len(verses)
	if res.Hits.Total != nil {
		total = int(res.Hits.Total.Value)
	}

	return total, verses, nil
}
