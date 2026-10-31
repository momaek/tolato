package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/attestation"
	"github.com/momaek/tolato/server/internal/config"
	"github.com/momaek/tolato/server/internal/middleware"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/store"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPatatoIntegration(t *testing.T) {
	dsn := os.Getenv("TOLATO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TOLATO_TEST_POSTGRES_DSN to a disposable PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	old := store.DB
	store.DB = tx
	defer func() { tx.Rollback(); store.DB = old }()
	if err := tx.AutoMigrate(&model.User{}, &model.APIKey{}, &model.Node{}, &model.Grant{}, &model.UserGroupMember{}, &model.NodeGroupMember{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	create := func(v any) {
		t.Helper()
		if err := tx.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []model.User{{ID: "p-admin", Username: "p-admin", Role: model.RoleAdmin}, {ID: "p-member", Username: "p-member", Role: model.RoleMember}} {
		create(&u)
	}
	for _, u := range []string{"p-admin", "p-member"} {
		for _, p := range []string{model.APIKeyReadonly, model.APIKeyWritable} {
			name := u + "-" + p
			create(&model.APIKey{ID: name, Name: name, OwnerUserID: u, KeyHash: middleware.HashAPIKey(name), Permission: p, Status: "active"})
		}
	}
	alias := "Tokyo"
	nodes := make([]model.Node, 0, 205)
	for i := 0; i < 205; i++ {
		nodes = append(nodes, model.Node{ID: fmt.Sprintf("p-%03d", i), Name: fmt.Sprintf("host-%03d", i), IP: fmt.Sprintf("192.0.2.%d", i), CountryCode: "JP", City: "Tokyo", ASN: "AS123", AgentSecret: "secret"})
	}
	nodes[204].Alias = &alias
	create(&nodes)
	create(&model.Grant{ID: "p-grant", SubjectType: model.SubjectUser, SubjectID: "p-member", ObjectType: model.ObjectNode, ObjectID: "p-204", Level: model.LevelViewer})
	deps := &Deps{Config: &config.Config{}, NodeManager: node.NewNodeManager()}
	router := SetupRouter(deps)
	request := func(method, path, key, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if key != "" {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	check := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("want %d got %d: %s", code, w.Code, w.Body.String())
		}
	}
	ro, rw, member := "p-admin-"+model.APIKeyReadonly, "p-admin-"+model.APIKeyWritable, "p-member-"+model.APIKeyReadonly
	check(request("GET", "/api/v1/nodes", "", ""), 401)
	w := request("GET", "/api/v1/nodes", ro, "")
	check(w, 200)
	var listed []model.NodeListItem
	if json.Unmarshal(w.Body.Bytes(), &listed) != nil || len(listed) != 205 {
		t.Fatal("fleet truncated")
	}
	for _, n := range listed {
		if n.CountryCode != "JP" || n.City != "Tokyo" || n.ASN != "AS123" {
			t.Fatal("missing geo")
		}
	}
	for _, path := range []string{"/api/v1/nodes?name=tOkYo", "/api/v1/nodes?ip=192.0.2.204"} {
		w = request("GET", path, member, "")
		check(w, 200)
		json.Unmarshal(w.Body.Bytes(), &listed)
		if len(listed) != 1 || listed[0].ID != "p-204" {
			t.Fatal("exact scoped filter failed")
		}
	}
	w = request("GET", "/api/v1/nodes?name=host-001", member, "")
	check(w, 200)
	if w.Body.String() != "[]" {
		t.Fatal("hidden node leaked")
	}
	w = request("GET", "/api/v1/nodes/p-204", member, "")
	check(w, 200)
	if !strings.Contains(w.Body.String(), `"country_code":"JP"`) {
		t.Fatal("detail missing geo")
	}
	check(request("GET", "/api/v1/nodes/p-001/commands", member, ""), 404)

	boundary := time.Now().UTC().Truncate(time.Second)
	output := "sensitive-output"
	metadata := map[string]string{"password": "sensitive-metadata"}
	create(&model.AuditLog{NodeID: "p-204", Command: "boundary", Source: "cli", CreatedAt: boundary})
	sources := []string{"cli", "api", "webui", "terminal", "terminal/file_op", "file_transfer", "service"}
	for _, source := range sources {
		create(&model.AuditLog{NodeID: "p-204", Command: source, Source: source, Actor: "alice", Stdout: &output, Stderr: &output, StdoutDigest: model.DigestOutput(output), Metadata: metadata, CreatedAt: boundary.Add(time.Second)})
	}
	create(&model.AuditLog{NodeID: "p-001", Command: "hidden-command", Source: "api", CreatedAt: boundary.Add(time.Second)})
	type feed struct {
		Items      []model.CommandEvent
		Next       string `json:"next_cursor"`
		Checkpoint string
		Total      int
	}
	read := func(path string) feed {
		t.Helper()
		w := request("GET", path, member, "")
		check(w, 200)
		for _, secret := range []string{"stdout", "stderr", "digest", "metadata", "sensitive", "hidden-command", "boundary"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("feed leaked %s: %s", secret, w.Body.String())
			}
		}
		var f feed
		if err := json.Unmarshal(w.Body.Bytes(), &f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	path := "/api/v1/nodes/p-204/commands?since=" + url.QueryEscape(boundary.Format(time.RFC3339Nano)) + "&page_size=2"
	f := read(path)
	if f.Total != len(sources) || len(f.Items) != 2 || f.Next == "" {
		t.Fatal("bad initial page")
	}
	// Appended equal-timestamp rows do not move the snapshot pages, but must
	// appear when the final checkpoint is polled.
	late := &model.AuditLog{NodeID: "p-204", Command: "late", Source: "api", CreatedAt: boundary.Add(time.Second)}
	create(late)
	seen := map[uint]bool{}
	for {
		for _, item := range f.Items {
			if seen[item.ID] {
				t.Fatal("duplicate across tied pages")
			}
			seen[item.ID] = true
			if item.Actor != "alice" {
				t.Fatal("actor lost")
			}
		}
		if f.Next == "" {
			break
		}
		f = read("/api/v1/nodes/p-204/commands?cursor=" + f.Next + "&page_size=2")
	}
	if len(seen) != len(sources) || f.Checkpoint == "" {
		t.Fatal("page loss or missing checkpoint")
	}
	f = read("/api/v1/nodes/p-204/commands?cursor=" + f.Checkpoint)
	if len(f.Items) != 1 || f.Items[0].ID != late.ID {
		t.Fatal("checkpoint lost equal-timestamp insert")
	}
	f = read("/api/v1/nodes/p-204/commands?cursor=" + f.Checkpoint)
	if len(f.Items) != 0 {
		t.Fatal("checkpoint repeated records")
	}
	for _, query := range []string{"since=bad", "page=0", "page_size=101", "cursor=bad"} {
		check(request("GET", "/api/v1/nodes/p-204/commands?"+query, member, ""), 400)
	}
	check(request("GET", "/api/v1/nodes/p-001/commands?cursor="+f.Checkpoint, ro, ""), 400)

	claims := attestation.Claims{NodeID: "p-204", Audience: "patato", Nonce: strings.Repeat("n", 32), IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Minute).Unix()}
	raw, _ := json.Marshal(claims)
	payload := "ta1." + base64.RawURLEncoding.EncodeToString(raw)
	derive := hmac.New(sha256.New, []byte("secret"))
	derive.Write([]byte("tolato/attestation/v1"))
	mac := hmac.New(sha256.New, derive.Sum(nil))
	mac.Write([]byte(payload))
	token := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	proof := func(aud string) string {
		raw, _ := json.Marshal(map[string]string{"token": token, "audience": aud, "nonce": claims.Nonce})
		return string(raw)
	}
	w = request("POST", "/api/v1/attestations/verify", member, proof("patato"))
	check(w, 200)
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("secret leaked")
	}
	check(request("POST", "/api/v1/attestations/verify", member, proof("other")), 400)
	check(request("POST", "/api/v1/attestations/verify", "", proof("patato")), 401)
	if err := tx.Model(&model.Grant{}).Where("id = ?", "p-grant").Update("object_id", "p-001").Error; err != nil {
		t.Fatal(err)
	}
	check(request("POST", "/api/v1/attestations/verify", member, proof("patato")), 404)
	if err := tx.Model(&model.Grant{}).Where("id = ?", "p-grant").Update("object_id", "p-204").Error; err != nil {
		t.Fatal(err)
	}

	servicePath := "/api/v1/nodes/p-204/services/patato-node"
	check(request("POST", servicePath, ro, `{"action":"restart"}`), 403)
	check(request("POST", servicePath, "p-member-"+model.APIKeyWritable, `{"action":"restart"}`), 404)
	check(request("GET", servicePath, member, ""), 503)
	up := websocket.Upgrader{}
	ready := make(chan *node.AgentConn, 1)
	calls := make(chan model.AgentServiceRequest, 4)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		ac := deps.NodeManager.RegisterConn("p-204", conn)
		ready <- ac
		<-ac.Done()
		deps.NodeManager.RemoveConn("p-204", ac)
	}))
	defer wsServer.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ac := <-ready
	check(request("GET", servicePath, member, ""), 501)
	ac.SetServicesVersion(1)
	go func() {
		for {
			var msg node.AgentFrame
			if conn.ReadJSON(&msg) != nil {
				return
			}
			var req model.AgentServiceRequest
			msg.Decode(&req)
			calls <- req
			_ = conn.WriteJSON(model.WSMessage{Type: "service_result", ID: msg.ID, Payload: model.AgentServiceResult{OK: true, Status: model.ServiceStatus{Enabled: true, Running: true}}})
		}
	}()
	w = request("GET", servicePath, member, "")
	check(w, 200)
	if req := <-calls; req.Action != "status" {
		t.Fatal("bad service read")
	}
	check(request("POST", servicePath, rw, `{"action":"restart"}`), 200)
	if req := <-calls; req.Action != "restart" || req.Service != "patato-node" {
		t.Fatal("bad service action")
	}
	var audit model.AuditLog
	if err := tx.Where("node_id = ? AND command = ?", "p-204", "service:restart patato-node").First(&audit).Error; err != nil {
		t.Fatal("service mutation not audited")
	}
	check(request("POST", servicePath, rw, `{"action":"unknown"}`), 400)
	if err := tx.Model(&model.APIKey{}).Where("id = ?", member).Update("status", "revoked").Error; err != nil {
		t.Fatal(err)
	}
	check(request("GET", servicePath, member, ""), 401)
}
