package ginhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterRegistersHealthzAndNodeRoutes(t *testing.T) {
	t.Parallel()

	router := NewRouter(Handler{Nodes: &fakeNodeViewService{}})

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	router.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", healthRec.Code)
	}

	nodesReq := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	nodesRec := httptest.NewRecorder()
	router.ServeHTTP(nodesRec, nodesReq)
	if nodesRec.Code != http.StatusOK {
		t.Fatalf("/api/v1/nodes status = %d, want 200", nodesRec.Code)
	}
}
