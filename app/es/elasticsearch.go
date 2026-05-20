package es

import (
	"bytes"
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
	verseProp := types.NewTextProperty()
	portuguese := "portuguese"
	verseProp.Analyzer = &portuguese
	mapping.Properties = map[string]types.Property{
		"book":    types.NewKeywordProperty(),
		"chapter": types.NewIntegerNumberProperty(),
		"number":  types.NewIntegerNumberProperty(),
		"verse":   verseProp,
	}

	return mapping
}

func IndexVerses(indexName string, verses []extract.VerseDocument, es *elasticsearch.TypedClient) error {
	const batchSize = 1000

	for start := 0; start < len(verses); start += batchSize {
		end := start + batchSize
		if end > len(verses) {
			end = len(verses)
		}

		var buf bytes.Buffer
		for _, verse := range verses[start:end] {
			docID := fmt.Sprintf("%s-%d-%d", verse.Book, verse.Chapter, verse.Number)
			meta := fmt.Sprintf(`{"index":{"_id":%q}}`+"\n", docID)
			buf.WriteString(meta)

			body, err := json.Marshal(verse)
			if err != nil {
				return fmt.Errorf("marshaling %s: %w", docID, err)
			}
			buf.Write(body)
			buf.WriteByte('\n')
		}

		res, err := es.Bulk().Index(indexName).Raw(&buf).Do(context.Background())
		if err != nil {
			return fmt.Errorf("bulk indexing batch starting %d: %w", start, err)
		}
		if res.Errors {
			for _, item := range res.Items {
				for _, op := range item {
					if op.Error != nil {
						return fmt.Errorf("bulk item failed: %v", *op.Error.Reason)
					}
				}
			}
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

func CheckESDataExistence(indexName string, es *elasticsearch.TypedClient) (bool, error) {
	exists, err := es.Indices.Exists(indexName).Do(context.Background())
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	res, err := es.Count().Index(indexName).Do(context.Background())
	if err != nil {
		return false, err
	}

	return res.Count > 0, nil
}
