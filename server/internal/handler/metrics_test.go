package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/nodemetrics"
	"github.com/momaek/tolato/server/internal/store"
	"github.com/prometheus/common/expfmt"
	prommodel "github.com/prometheus/common/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// A real WebSocket feeds the cache, so tests cover dispatch and serialization,
// not just a hand-built cache which could bypass identity validation.
func connectMetricsNode(t *testing.T, manager *node.NodeManager, id, data string) (*websocket.Conn, *node.AgentConn) {
	t.Helper()
	up := websocket.Upgrader{}
	ready := make(chan *node.AgentConn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		ac := manager.RegisterConn(id, conn, func(ac *node.AgentConn) {
			ac.ConfigureExporter(model.AgentRegisterPayload{NodeExporterEnabled: true, MetricsIntervalSeconds: 30})
		})
		ready <- ac
		<-ac.Done()
		manager.RemoveConn(id, ac)
	}))
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ac := <-ready
	sendMetrics(t, conn, ac, data)
	return conn, ac
}
func sendMetrics(t *testing.T, conn *websocket.Conn, ac *node.AgentConn, data string) {
	t.Helper()
	before := ac.ExporterState().ReceivedAt
	if err := conn.WriteJSON(model.WSMessage{Type: "node_metrics", Payload: nodemetrics.Snapshot{Version: 1, CollectedAt: float64(time.Now().Unix()), Data: data}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ac.ExporterState().ReceivedAt.After(before) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("snapshot not cached")
}
func TestMetricsEndpointFreshnessAndConflicts(t *testing.T) {
	manager := node.NewNodeManager()
	conn, ac := connectMetricsNode(t, manager, "a", "# HELP node_load1 Load\n# TYPE node_load1 gauge\nnode_load1 2\n")
	_, _ = connectMetricsNode(t, manager, "b", "# HELP node_load1 Conflicting help\n# TYPE node_load1 gauge\nnode_load1 3\n")
	nodes := []model.Node{{ID: "a", Name: "host-a"}, {ID: "b", Name: "host-b"}}
	deps := &Deps{Config: &config.Config{}, NodeManager: manager}
	handler := externalMetrics(deps, func(*gin.Context) ([]model.Node, error) { return nodes, nil })
	scrape := func() string {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("GET", "/api/v1/metrics", nil)
		handler(ctx)
		if w.Code != http.StatusOK {
			t.Fatalf("scrape status %d: %s", w.Code, w.Body.String())
		}
		parser := expfmt.NewTextParser(prommodel.LegacyValidation)
		if _, err := parser.TextToMetricFamilies(strings.NewReader(w.Body.String())); err != nil {
			t.Fatal(err)
		}
		return w.Body.String()
	}
	body := scrape()
	if !strings.Contains(body, `node_load1{node_id="a"} 2`) || strings.Contains(body, `node_load1{node_id="b"}`) {
		t.Fatalf("conflict not isolated: %s", body)
	}
	if !strings.Contains(body, `tolato_node_metrics_export_success{node_id="b"} 0`) {
		t.Fatal("schema conflict not observable")
	}
	// A stale cache is omitted while status series remain.
	expired := gatherNodeMetrics(manager, nodes, ac.ExporterState().ReceivedAt.Add(91*time.Second), 0)
	for _, f := range expired {
		if f.GetName() == "node_load1" {
			t.Fatal("expired metrics exported")
		}
	}
	conn.Close()
	select {
	case <-ac.Done():
	case <-time.After(time.Second):
		t.Fatal("socket did not close")
	}
	if strings.Contains(scrape(), `node_load1{node_id="a"}`) {
		t.Fatal("disconnected cache exported")
	}
	// The visibility resolver is called for each scrape; metadata cannot leak
	// merely because a connection was present in the in-memory manager.
	nodes = nil
	if body := scrape(); strings.Contains(body, "host-a") || strings.Contains(body, "node_id=") {
		t.Fatal("hidden node leaked")
	}
}

// Run against a dedicated disposable PostgreSQL database, never a deployment DB:
// TOLATO_TEST_POSTGRES_DSN='host=127.0.0.1 ...' go test ./internal/handler -run TestMetricsAuthorizationIntegration
func TestMetricsAuthorizationIntegration(t *testing.T) {
	dsn := os.Getenv("TOLATO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TOLATO_TEST_POSTGRES_DSN to a disposable PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	old := store.DB
	store.DB = tx
	t.Cleanup(func() { tx.Rollback(); store.DB = old })
	if err := tx.AutoMigrate(&model.User{}, &model.APIKey{}, &model.Node{}, &model.Grant{}, &model.UserGroupMember{}, &model.NodeGroupMember{}); err != nil {
		t.Fatal(err)
	}
	create := func(value any) {
		t.Helper()
		if err := tx.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	create(&model.User{ID: "admin", Username: "metrics-admin", Role: model.RoleAdmin, Status: model.UserStatusActive})
	create(&model.User{ID: "member", Username: "metrics-member", Role: model.RoleMember, Status: model.UserStatusActive})
	for _, u := range []string{"admin", "member"} {
		create(&model.APIKey{ID: u, Name: u, OwnerUserID: u, KeyHash: middleware.HashAPIKey("test-" + u), Permission: model.APIKeyReadonly, Status: "active"})
	}
	var nodes []model.Node
	for i := 0; i < 205; i++ {
		nodes = append(nodes, model.Node{ID: fmt.Sprintf("node-%03d", i), Name: fmt.Sprintf("host-%03d", i)})
	}
	create(&nodes)
	create(&model.Grant{ID: "grant", SubjectType: model.SubjectUser, SubjectID: "member", ObjectType: model.ObjectNode, ObjectID: "node-000", Level: model.LevelViewer})
	manager := node.NewNodeManager()
	connectMetricsNode(t, manager, "node-000", "node_load1 1\n")
	connectMetricsNode(t, manager, "node-001", "node_load1 99\n")
	deps := &Deps{Config: &config.Config{}, NodeManager: manager}
	router := SetupRouter(deps)
	scrape := func(token string) (int, string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/v1/metrics", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	if code, _ := scrape(""); code != http.StatusUnauthorized {
		t.Fatal("unauthenticated scrape accepted")
	}
	code, body := scrape("test-member")
	if code != 200 || !strings.Contains(body, `node_load1{node_id="node-000"} 1`) || strings.Contains(body, "node-001") || strings.Contains(body, "host-001") {
		t.Fatalf("member visibility broken: %d %s", code, body)
	}
	code, body = scrape("test-admin")
	if code != 200 || !strings.Contains(body, "host-204") || !strings.Contains(body, `node_load1{node_id="node-001"} 99`) {
		t.Fatal("admin scrape truncated or missing nodes")
	}
	if err := tx.Delete(&model.Grant{}, "id = ?", "grant").Error; err != nil {
		t.Fatal(err)
	}
	code, body = scrape("test-member")
	if code != 200 || strings.Contains(body, "node-000") {
		t.Fatal("revoked node permission still exposes metrics")
	}
	if err := tx.Model(&model.APIKey{}).Where("id = ?", "member").Update("status", "revoked").Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := scrape("test-member"); code != 401 {
		t.Fatal("revoked key accepted")
	}
	if err := tx.Delete(&model.Node{}, "id = ?", "node-000").Error; err != nil {
		t.Fatal(err)
	}
	code, body = scrape("test-admin")
	if code != 200 || strings.Contains(body, "node-000") {
		t.Fatal("deleted node's active cache leaked")
	}

	if err := tx.Model(&model.User{}).Where("id = ?", "admin").Update("status", model.UserStatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := scrape("test-admin"); code != 401 {
		t.Fatal("disabled owner accepted")
	}
}
