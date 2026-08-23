package agentdock

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestHubInvokesConnectedNode(t *testing.T) {
	store, _ := newTestStore(t)
	pairing, err := store.CreatePairingCode(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.Pair(t.Context(), PairInput{Code: pairing.Code, DeviceID: "device_12345678", Name: "DockMini"})
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(store)
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hub.Accept(w, r, node.ID); err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		close(connected)
	}))
	defer server.Close()

	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	if err := socket.WriteJSON(connectionMessage{Type: "node.hello", Protocol: ConnectionProtocolVersion, Hello: &Hello{
		DeviceID: node.DeviceID, ProtocolVersion: ConnectionProtocolVersion, Capabilities: []string{"server_info"},
	}}); err != nil {
		t.Fatal(err)
	}
	var ready connectionMessage
	if err := socket.ReadJSON(&ready); err != nil || ready.Type != "node.ready" {
		t.Fatalf("ready=%#v err=%v", ready, err)
	}
	<-connected

	done := make(chan error, 1)
	go func() {
		var invoke connectionMessage
		if err := socket.ReadJSON(&invoke); err != nil {
			done <- err
			return
		}
		done <- socket.WriteJSON(connectionMessage{Type: "tool.result", RequestID: invoke.RequestID, Result: []byte(`{"ok":true}`)})
	}()
	result, err := hub.Invoke(context.Background(), node.ID, "tool.call", map[string]any{"tool": "server_info"})
	if err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true {
		t.Fatalf("result = %#v", result)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
