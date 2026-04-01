package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/momaek/tolato/internal/nodeprobe/model"
)

// Reporter sends metric reports to the NodeProbe server.
type Reporter struct {
	serverURL string
	authToken string
	nodeID    string
	client    *http.Client
	logger    *log.Logger
}

// NewReporter creates a Reporter.
func NewReporter(serverURL, authToken, nodeID string, logger *log.Logger) *Reporter {
	return &Reporter{
		serverURL: serverURL,
		authToken: authToken,
		nodeID:    nodeID,
		client:    &http.Client{Timeout: 10 * time.Second},
		logger:    logger,
	}
}

// Report sends a batch of target metrics to the server.
func (r *Reporter) Report(ctx context.Context, metrics []model.TargetMetric) error {
	report := model.MetricReport{
		NodeID:    r.nodeID,
		Timestamp: time.Now(),
		Metrics:   metrics,
	}

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.serverURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.authToken)

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("send report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("report rejected: HTTP %d", resp.StatusCode)
	}

	r.logger.Printf("reported %d metrics to server", len(metrics))
	return nil
}
