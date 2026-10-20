package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateMetadata(t *testing.T) {
	tooMany := map[string]string{}
	for i := 0; i <= maxMetadataKeys; i++ {
		tooMany["k"+strings.Repeat("x", i)] = "v"
	}

	tests := []struct {
		name string
		in   map[string]string
		ok   bool
	}{
		{"absent", nil, true},
		{"typical", map[string]string{"ticket": "T-42", "run.id": "a1", "on_behalf-of": "李雷"}, true},
		{"value at the limit", map[string]string{"k": strings.Repeat("v", maxMetadataValueLen)}, true},
		{"value over the limit", map[string]string{"k": strings.Repeat("v", maxMetadataValueLen+1)}, false},
		{"empty key", map[string]string{"": "v"}, false},
		{"key over the limit", map[string]string{strings.Repeat("k", maxMetadataKeyLen+1): "v"}, false},
		// These would not survive as a metadata.<key> query parameter.
		{"key with a space", map[string]string{"a b": "v"}, false},
		{"key with &", map[string]string{"a&b": "v"}, false},
		{"key with =", map[string]string{"a=b": "v"}, false},
		{"NUL in value", map[string]string{"k": "a\x00b"}, false},
		{"invalid UTF-8", map[string]string{"k": "\xff"}, false},
		{"too many keys", tooMany, false},
	}
	for _, tt := range tests {
		err := validateMetadata(tt.in)
		if (err == nil) != tt.ok {
			t.Errorf("%s: validateMetadata = %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
}

func TestMetadataFilter(t *testing.T) {
	ctx := func(query string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/api/audit-logs?"+query, nil)
		return c
	}

	got, err := metadataFilter(ctx("page=2&keyword=ls&metadata.ticket=T-42&metadata.run.id=a%201"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["ticket"] != "T-42" || got["run.id"] != "a 1" {
		t.Errorf("got %v", got)
	}

	// No metadata parameters means no filter at all, not an empty one.
	if got, err := metadataFilter(ctx("keyword=ls")); err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}

	if _, err := metadataFilter(ctx("metadata.=x")); err == nil {
		t.Error("accepted an empty metadata key")
	}
}
