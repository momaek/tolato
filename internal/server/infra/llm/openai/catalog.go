package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/momaek/tolato/internal/server/domain"
)

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p Provider) ListModels(ctx context.Context) ([]string, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, domain.ErrInvalidArgument
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint()+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(p.APIKey))

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decoded modelsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if decoded.Error != nil && decoded.Error.Message != "" {
			return nil, errors.New(decoded.Error.Message)
		}
		return nil, fmt.Errorf("openai models request failed with status %d", resp.StatusCode)
	}

	seen := make(map[string]struct{}, len(decoded.Data))
	models := make([]string, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	slices.Sort(models)
	return models, nil
}
