package enroll

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/momaek/tolato/internal/shared/types"
)

type Client interface {
	Enroll(ctx context.Context, req types.EnrollRequest) (types.EnrollResponse, error)
}

type HTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewClient(baseURL string) HTTPClient {
	return HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (c HTTPClient) Enroll(ctx context.Context, req types.EnrollRequest) (types.EnrollResponse, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return types.EnrollResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agent/enroll", bytes.NewReader(raw))
	if err != nil {
		return types.EnrollResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return types.EnrollResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return types.EnrollResponse{}, fmt.Errorf("enroll failed with status %d", resp.StatusCode)
	}

	var out types.EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return types.EnrollResponse{}, err
	}

	return out, nil
}
