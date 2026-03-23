package settings

import (
	"context"

	appsettings "github.com/momaek/tolato/internal/server/app/settings"
	"github.com/momaek/tolato/internal/server/domain"
	openai "github.com/momaek/tolato/internal/server/infra/llm/openai"
)

type Catalog struct{}

func (c Catalog) ListModels(ctx context.Context, provider string, endpoint string, apiKey string) ([]appsettings.ModelOption, error) {
	switch normalizeProvider(provider) {
	case "openai":
		items, err := (openai.Provider{
			Endpoint: endpoint,
			APIKey:   apiKey,
		}).ListModels(ctx)
		if err != nil {
			return nil, err
		}

		models := make([]appsettings.ModelOption, 0, len(items))
		for _, item := range items {
			models = append(models, appsettings.ModelOption{
				ID:    item,
				Label: item,
			})
		}
		return models, nil
	case "devloop":
		return []appsettings.ModelOption{{
			ID:    "devloop",
			Label: "devloop",
		}}, nil
	default:
		return nil, domain.ErrUnsupportedConfig
	}
}
