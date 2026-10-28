package nodeexporter

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/node_exporter/collector"
)

func TestUpstreamCollectorCompatibility(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	embedded, err := New(logger)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := collector.NewNodeCollector(logger)
	if err != nil {
		t.Fatal(err)
	}
	registry := prometheus.NewRegistry()
	registry.MustRegister(reference)
	upstream, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	text, _, err := embedded.Gather(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parser := expfmt.NewTextParser(prommodel.LegacyValidation)
	actual, err := parser.TextToMetricFamilies(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range upstream {
		got := actual[f.GetName()]
		if got == nil || got.GetType() != f.GetType() || got.GetHelp() != f.GetHelp() {
			t.Errorf("upstream schema mismatch: %s", f.GetName())
			continue
		}
		expectedLabels := labelNames(f.Metric[0])
		actualLabels := labelNames(got.Metric[0])
		if expectedLabels != actualLabels {
			t.Errorf("%s labels: %s, want %s", f.GetName(), actualLabels, expectedLabels)
		}
	}
	build := actual["node_exporter_build_info"]
	found := false
	for _, l := range build.Metric[0].Label {
		if l.GetName() == "version" && l.GetValue() == Version {
			found = true
		}
	}
	if !found {
		t.Fatal("embedded source version missing")
	}
}
func labelNames(m *dto.Metric) string {
	var names []string
	for _, l := range m.Label {
		names = append(names, l.GetName())
	}
	return strings.Join(names, ",")
}

type stuckGatherer struct{ release chan struct{} }

func (g stuckGatherer) Gather() ([]*dto.MetricFamily, error) { <-g.release; return nil, nil }
func TestTimeoutDoesNotAccumulateCollectors(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	e := &Exporter{registry: stuckGatherer{release}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, _, err := e.Gather(ctx); err == nil {
		t.Fatal("stuck collector did not time out")
	}
	for range 10 {
		if _, _, err := e.Gather(context.Background()); err == nil || !strings.Contains(err.Error(), "still running") {
			t.Fatalf("started another stuck collection: %v", err)
		}
	}
}

// Use a subprocess because upstream collector flags/state are process-global.
func TestTextfileFlags(t *testing.T) {
	if os.Getenv("TOLATO_EXPORTER_TEST_HELPER") == "1" {
		_, err := kingpin.CommandLine.Parse([]string{"--collector.disable-defaults", "--collector.textfile", "--collector.textfile.directory=" + os.Getenv("TOLATO_EXPORTER_TEST_DIR")})
		if err != nil {
			t.Fatal(err)
		}
		exporter, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		text, _, err := exporter.Gather(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "custom_fixture_metric{service=\"example\"} 42") {
			t.Fatal("textfile collector flags were not honored")
		}
		if strings.Contains(text, "node_cpu_seconds_total") {
			t.Fatal("disable-defaults not honored")
		}
		return
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fixture.prom"), []byte("# TYPE custom_fixture_metric gauge\ncustom_fixture_metric{service=\"example\"} 42\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestTextfileFlags$")
	cmd.Env = append(os.Environ(), "TOLATO_EXPORTER_TEST_HELPER=1", "TOLATO_EXPORTER_TEST_DIR="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("textfile subprocess: %v\n%s", err, output)
	}
}
