package store

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestCommandWatermarkWaitsForCommit(t *testing.T) {
	dsn := os.Getenv("TOLATO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TOLATO_TEST_POSTGRES_DSN to a disposable PostgreSQL database")
	}
	ns := fmt.Sprintf("tolato_audit_%d", time.Now().UnixNano())
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: ns + "."}})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.Exec("CREATE SCHEMA " + ns).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + ns + " CASCADE")
	old := DB
	DB = db
	defer func() { DB = old }()
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	node := "watermark-" + time.Now().Format("150405.000000000")
	defer db.Where("node_id = ?", node).Delete(&model.AuditLog{})
	allocated, release := make(chan struct{}), make(chan struct{})
	// Delay one real insert after it has allocated an ID but before commit.
	if err := db.Callback().Create().After("gorm:create").Register("test:delay-commit", func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*model.AuditLog); ok && row.NodeID == node && row.Command == "slow" {
			close(allocated)
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove("test:delay-commit")
	slow := &model.AuditLog{NodeID: node, Command: "slow"}
	firstDone := make(chan error, 1)
	go func() { firstDone <- CreateAuditLog(slow) }()
	select {
	case <-allocated:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("insert did not reach commit boundary")
	}
	fast := &model.AuditLog{NodeID: node, Command: "fast"}
	secondDone := make(chan error, 1)
	go func() { secondDone <- CreateAuditLog(fast) }()
	type result struct {
		id  uint
		err error
	}
	watermark := make(chan result, 1)
	go func() { id, err := LatestNodeAuditID(node); watermark <- result{id, err} }()
	select {
	case got := <-watermark:
		close(release)
		<-firstDone
		<-secondDone
		t.Fatalf("watermark overtook uncommitted audit: %+v", got)
	case err := <-secondDone:
		close(release)
		<-firstDone
		<-watermark
		t.Fatalf("later write overtook uncommitted audit: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	got := <-watermark
	if got.err != nil || got.id < slow.ID {
		t.Fatalf("bad committed watermark %+v", got)
	}
	items, _, err := ListNodeCommandEvents(node, CommandEventWindow{UpperID: got.id}, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	seenSlow := false
	for _, row := range items {
		if row.ID == slow.ID {
			seenSlow = true
		}
	}
	if !seenSlow {
		t.Fatal("committed slow row missing below checkpoint")
	}
	// Whichever side of the snapshot the fast insert lands on, it must appear
	// either in that window or in the next polling window, exactly once.
	last, err := LatestNodeAuditID(node)
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := ListNodeCommandEvents(node, CommandEventWindow{LowerID: got.id, UpperID: last}, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uint]bool{}
	for _, row := range append(items, next...) {
		if seen[row.ID] {
			t.Fatal("duplicate across committed windows")
		}
		seen[row.ID] = true
	}
	if len(seen) != 2 {
		t.Fatal("concurrent insert was lost")
	}
}
