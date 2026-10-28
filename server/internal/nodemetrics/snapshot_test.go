package nodemetrics

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"

	"strings"
	"testing"
)

func TestSnapshotValidation(t *testing.T) {
	valid := "# HELP node_cpu_seconds_total CPU time\n# TYPE node_cpu_seconds_total counter\nnode_cpu_seconds_total{cpu=\"0\",mode=\"idle\"} 123\n"
	fs, err := Parse(Snapshot{Version: Version, CollectedAt: 100, Data: valid}, "trusted-node")
	if err != nil {
		t.Fatal(err)
	}
	labels := fs[0].Metric[0].Label
	found := false
	for _, l := range labels {
		if l.GetName() == "node_id" && l.GetValue() == "trusted-node" {
			found = true
		}
	}
	if !found {
		t.Fatal("authenticated node identity missing")
	}
	for name, data := range map[string]string{
		"spoofed identity":   "metric{node_id=\"other\"} 1\n",
		"reserved namespace": "tolato_node_connected 1\n",
		"duplicate":          "metric{a=\"b\"} 1\nmetric{a=\"b\"} 2\n",
		"timestamp":          "metric 1 1234\n",
		"malformed":          "metric{ 1\n",
		"oversize":           strings.Repeat(" ", MaxBytes+1),
		"empty":              "",
		"suffix collision":   "# TYPE metric histogram\nmetric_bucket{le=\"+Inf\"} 1\nmetric_sum 1\nmetric_count 1\n# TYPE metric_sum gauge\nmetric_sum 2\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(Snapshot{Version: Version, CollectedAt: 100, Data: data}, "trusted-node"); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
	for _, snapshot := range []Snapshot{{Version: 2, CollectedAt: 100, Data: valid}, {Version: 1, Data: valid}, {Version: 1, CollectedAt: 100, Data: valid, Error: "timeout"}} {
		if _, err := Parse(snapshot, "n"); err == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
}

func TestSchemaRejectsCrossNodeConflicts(t *testing.T) {
	first, err := Parse(Snapshot{Version: 1, CollectedAt: 100, Data: "# HELP metric One\n# TYPE metric gauge\nmetric 1\n"}, "a")
	if err != nil {
		t.Fatal(err)
	}
	schema := NewSchema()
	if !schema.Add(first) {
		t.Fatal("first schema rejected")
	}
	same, _ := Parse(Snapshot{Version: 1, CollectedAt: 100, Data: "# HELP metric One\n# TYPE metric gauge\nmetric 2\n"}, "b")
	if !schema.Add(same) {
		t.Fatal("compatible schema rejected")
	}
	conflicting, _ := Parse(Snapshot{Version: 1, CollectedAt: 100, Data: "# HELP metric Other\n# TYPE metric gauge\nmetric 2\n"}, "c")
	if schema.Add(conflicting) {
		t.Fatal("incompatible help accepted")
	}
}

func TestCompressedSnapshots(t *testing.T) {
	encode := func(text string) string {
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		if _, err := writer.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(buffer.Bytes())
	}
	snapshot := Snapshot{Version: 1, CollectedAt: 100, Encoding: "gzip+base64", Data: encode("node_load1 2\n")}
	if _, err := Parse(snapshot, "n"); err != nil {
		t.Fatal(err)
	}
	snapshot.Data = encode(strings.Repeat(" ", MaxBytes+1))
	if _, err := Parse(snapshot, "n"); err == nil {
		t.Fatal("decompression limit bypassed")
	}
	snapshot.Data = "not-base64"
	if _, err := Parse(snapshot, "n"); err == nil {
		t.Fatal("invalid compressed data accepted")
	}
	snapshot.Encoding = "unknown"
	if _, err := Parse(snapshot, "n"); err == nil {
		t.Fatal("unknown encoding accepted")
	}
}
