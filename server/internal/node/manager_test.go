package node

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Regression tests for momaek/tolato#15: the server used to keep a dead
// agent socket in the NodeManager indefinitely because its read loop had no
// deadline and nothing ever pinged the peer.

func setAgentTiming(t *testing.T, ping, pong time.Duration) {
	t.Helper()
	op, ow := agentPingPeriod, agentPongWait
	agentPingPeriod, agentPongWait = ping, pong
	t.Cleanup(func() { agentPingPeriod, agentPongWait = op, ow })
}

// serveOne runs an httptest server that registers the first upgraded socket
// with nm under nodeID and delivers the resulting AgentConn on the returned
// channel.
func serveOne(t *testing.T, nm *NodeManager, nodeID string) (wsURL string, acCh <-chan *AgentConn) {
	t.Helper()
	ch := make(chan *AgentConn, 1)
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ac := nm.RegisterConn(nodeID, conn)
		ch <- ac
		<-ac.Done()
		nm.RemoveConn(nodeID)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), ch
}

func dial(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// A peer that never reads never Pongs; the router must give up on it after
// agentPongWait instead of holding the slot forever.
func TestAgentConn_ClosesWhenPeerStopsAnswering(t *testing.T) {
	setAgentTiming(t, 50*time.Millisecond, 200*time.Millisecond)
	nm := NewNodeManager()
	wsURL, acCh := serveOne(t, nm, "n1")

	_ = dial(t, wsURL) // never read → gorilla's auto-Pong never runs

	ac := <-acCh
	select {
	case <-ac.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("router kept a non-responsive socket open")
	}
	// The handler's RemoveConn runs after Done() closes, so allow it a moment.
	deadline := time.Now().Add(time.Second)
	for {
		if _, ok := nm.GetConn("n1"); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("dead connection still registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A peer that merely sits in ReadMessage answers Pings automatically and must
// be left alone well past agentPongWait.
func TestAgentConn_StaysUpWhilePeerPongs(t *testing.T) {
	setAgentTiming(t, 50*time.Millisecond, 200*time.Millisecond)
	nm := NewNodeManager()
	wsURL, acCh := serveOne(t, nm, "n1")

	client := dial(t, wsURL)
	go func() {
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ac := <-acCh
	select {
	case <-ac.Done():
		t.Fatal("router dropped a healthy, ponging socket")
	case <-time.After(4 * agentPongWait):
	}

	// The offline monitor's escape hatch: closing by node id ends run().
	if !nm.CloseConn("n1") {
		t.Fatal("CloseConn found no connection")
	}
	select {
	case <-ac.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("CloseConn did not terminate the router")
	}
}
