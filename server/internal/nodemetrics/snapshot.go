// Package nodemetrics validates the node_exporter wire format before caching it.
package nodemetrics

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"
)

const Version = 1
const MaxBytes = 4 << 20
const MaxSamples = 50000

type Snapshot struct {
	Encoding    string  `json:"encoding,omitempty"`
	Version     int     `json:"version"`
	CollectedAt float64 `json:"collected_at"`
	Data        string  `json:"data,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// Parse rejects conflicting/duplicate samples, reserved names, spoofed node IDs,
// and explicit sample timestamps. The server owns identity and scrape timestamps.
func Parse(s Snapshot, nodeID string) ([]*dto.MetricFamily, error) {
	if s.Version != Version {
		return nil, fmt.Errorf("unsupported metrics version %d", s.Version)
	}
	if len(s.Data) > MaxBytes || len(s.Error) > 1024 {
		return nil, fmt.Errorf("snapshot exceeds size limit")
	}
	if s.Error != "" {
		return nil, fmt.Errorf("agent collection failed: %s", s.Error)
	}
	if s.CollectedAt <= 0 || math.IsNaN(s.CollectedAt) || math.IsInf(s.CollectedAt, 0) {
		return nil, fmt.Errorf("invalid collection time")
	}
	text, err := decodeText(s)
	if err != nil {
		return nil, err
	}

	parser := expfmt.NewTextParser(prommodel.LegacyValidation)
	parsed, err := parser.TextToMetricFamilies(strings.NewReader(text))
	if err != nil {
		return nil, err
	}
	if len(parsed) == 0 {
		return nil, fmt.Errorf("empty snapshot")
	}
	families := make([]*dto.MetricFamily, 0, len(parsed))
	samples := 0
	for name, f := range parsed {
		if strings.HasPrefix(name, "tolato_") {
			return nil, fmt.Errorf("reserved metric name %q", name)
		}
		for _, m := range f.Metric {
			if m.TimestampMs != nil {
				return nil, fmt.Errorf("explicit sample timestamps are unsupported")
			}
			for _, l := range m.Label {
				if l.GetName() == "node_id" {
					return nil, fmt.Errorf("node_id is a reserved label")
				}
			}
			idName, idValue := "node_id", nodeID
			m.Label = append(m.Label, &dto.LabelPair{Name: &idName, Value: &idValue})
			samples++
			if h := m.Histogram; h != nil {
				samples += len(h.Bucket) + 1
			}
			if q := m.Summary; q != nil {
				samples += len(q.Quantile) + 1
			}
			if samples > MaxSamples {
				return nil, fmt.Errorf("snapshot exceeds %d samples", MaxSamples)
			}
		}
		families = append(families, f)
	}
	// Gatherers performs Prometheus's own metric/type/duplicate/suffix checks.
	return prometheus.Gatherers{prometheus.GathererFunc(func() ([]*dto.MetricFamily, error) { return families, nil })}.Gather()
}

// Schema tracks family and flattened sample names to isolate incompatible
// textfile metrics or node_exporter versions without breaking other nodes.
type Schema struct {
	families map[string]string
	samples  map[string]string
}

func NewSchema() *Schema { return &Schema{families: map[string]string{}, samples: map[string]string{}} }
func (s *Schema) Add(fs []*dto.MetricFamily) bool {
	for _, f := range fs {
		name := f.GetName()
		signature := fmt.Sprintf("%d:%s", f.GetType(), f.GetHelp())
		if previous, ok := s.families[name]; ok && previous != signature {
			return false
		}
		for _, sample := range sampleNames(f) {
			if owner, ok := s.samples[sample]; ok && owner != name {
				return false
			}
		}
	}
	for _, f := range fs {
		name := f.GetName()
		s.families[name] = fmt.Sprintf("%d:%s", f.GetType(), f.GetHelp())
		for _, sample := range sampleNames(f) {
			s.samples[sample] = name
		}
	}
	return true
}
func sampleNames(f *dto.MetricFamily) []string {
	n := f.GetName()
	switch f.GetType() {
	case dto.MetricType_HISTOGRAM:
		return []string{n, n + "_bucket", n + "_sum", n + "_count"}
	case dto.MetricType_SUMMARY:
		return []string{n, n + "_sum", n + "_count"}
	default:
		return []string{n}
	}
}

func Sorted(fs map[string]*dto.MetricFamily) []*dto.MetricFamily {
	names := make([]string, 0, len(fs))
	for n := range fs {
		names = append(names, n)
	}
	sort.Strings(names)
	result := make([]*dto.MetricFamily, 0, len(names))
	for _, n := range names {
		result = append(result, fs[n])
	}
	return result
}

func decodeText(s Snapshot) (string, error) {
	switch s.Encoding {
	case "":
		return s.Data, nil
	case "gzip+base64":
		compressed, err := base64.StdEncoding.DecodeString(s.Data)
		if err != nil {
			return "", fmt.Errorf("invalid metrics base64: %w", err)
		}
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return "", fmt.Errorf("invalid metrics gzip: %w", err)
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, MaxBytes+1))
		if err != nil {
			return "", fmt.Errorf("decompress metrics: %w", err)
		}
		if len(data) > MaxBytes {
			return "", fmt.Errorf("decompressed snapshot exceeds %d bytes", MaxBytes)
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("unsupported metrics encoding %q", s.Encoding)
	}
}
