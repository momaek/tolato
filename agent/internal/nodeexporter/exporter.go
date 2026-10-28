// Package nodeexporter embeds upstream collectors without an HTTP listener or a
// second process. Collector flags are registered by upstream on kingpin.CommandLine.
package nodeexporter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/version"
	"github.com/prometheus/node_exporter/collector"
)

const Version = "1.12.1"
const MaxSnapshotBytes = 4 << 20
const MaxSamples = 50000

var disableExporterMetrics = kingpin.Flag("web.disable-exporter-metrics", "Exclude Go/process self-metrics of the combined tolato-agent process.").Default("false").Bool()

var disableDefaults = kingpin.Flag("collector.disable-defaults", "Disable default node_exporter collectors; explicitly enable the desired collectors.").Default("false").Bool()

// Exporter owns a dedicated registry. At most one Gather may run, including
// after a timeout: upstream collectors do not all support cancellation.
type Exporter struct {
	registry prometheus.Gatherer
	busy     sync.Mutex
}

func New(logger *slog.Logger) (*Exporter, error) {
	if *disableDefaults {
		collector.DisableDefaultCollectors()
	}
	nc, err := collector.NewNodeCollector(logger)
	if err != nil {
		return nil, err
	}
	r := prometheus.NewRegistry()
	if err := r.Register(nc); err != nil {
		return nil, err
	}
	// common/version is used only by the embedded exporter in this binary.
	// Retain the upstream build_info schema and report its pinned source version.
	version.Version = Version
	r.MustRegister(versioncollector.NewCollector("node_exporter"))
	if !*disableExporterMetrics {
		r.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	}
	return &Exporter{registry: r}, nil
}

// Gather applies a caller deadline without starting another collection while a
// slow upstream collector is still running. Results after the deadline are discarded.
func (e *Exporter) Gather(ctx context.Context) (string, time.Time, error) {
	if !e.busy.TryLock() {
		return "", time.Time{}, errors.New("previous node_exporter collection is still running")
	}
	type result struct {
		data string
		at   time.Time
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer e.busy.Unlock()
		families, err := e.registry.Gather()
		if err != nil {
			done <- result{err: err}
			return
		}
		var b bytes.Buffer
		w := &limitedWriter{w: &b, remaining: MaxSnapshotBytes}
		samples := 0
		for _, family := range families {
			for _, metric := range family.Metric {
				samples++
				if h := metric.Histogram; h != nil {
					samples += len(h.Bucket) + 1
					if len(h.Bucket) == 0 || !math.IsInf(h.Bucket[len(h.Bucket)-1].GetUpperBound(), 1) {
						samples++
					}
				}
				if s := metric.Summary; s != nil {
					samples += len(s.Quantile) + 1
				}
			}
			if samples > MaxSamples {
				done <- result{err: fmt.Errorf("snapshot exceeds %d samples", MaxSamples)}
				return
			}
			if _, err := expfmt.MetricFamilyToText(w, family); err != nil {
				done <- result{err: err}
				return
			}
		}
		done <- result{data: b.String(), at: time.Now()}
	}()
	select {
	case r := <-done:
		return r.data, r.at, r.err
	case <-ctx.Done():
		return "", time.Time{}, ctx.Err()
	}
}

type limitedWriter struct {
	w         io.Writer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, fmt.Errorf("snapshot exceeds %d bytes", MaxSnapshotBytes)
	}
	n, err := w.w.Write(p)
	w.remaining -= n
	return n, err
}
