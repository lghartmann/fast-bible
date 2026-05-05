package elasticsearch

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/lghartmann/fast-bible/app/extract"
)

func NewElasticSearch(addr string) *elasticsearch.TypedClient {
	es, err := elasticsearch.NewTyped(
		elasticsearch.WithAddresses(addr),
		elasticsearch.WithRetry(3, http.StatusInternalServerError),
		elasticsearch.WithLogger(&elastictransport.TextLogger{Output: os.Stdout}),
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
