package agentdock

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/uvwt/nexusdock/internal/core"
)

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := core.OpenSQLite(context.Background(), ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.EnsureSchema(context.Background(), db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	store, err := NewStore(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func TestPairingCodeIsSingleUseAndStoresNoSecret(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	pairing, err := store.CreatePairingCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.Pair(ctx, PairInput{Code: pairing.Code, DeviceID: "device_12345678", Name: "DockMini"})
	if err != nil {
		t.Fatal(err)
	}
	if node.DeviceID != "device_12345678" || !node.Enabled {
		t.Fatalf("unexpected node: %#v", node)
	}
	if _, err := store.Pair(ctx, PairInput{Code: pairing.Code, DeviceID: "device_abcdefgh", Name: "Other"}); !errors.Is(err, ErrPairingCodeInvalid) {
		t.Fatalf("reused pairing code error = %v", err)
	}
	var legacyColumns int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('agentdock_devices') WHERE name IN ('endpoint', 'token')`).Scan(&legacyColumns); err != nil {
		t.Fatal(err)
	}
	if legacyColumns != 0 {
		t.Fatalf("agentdock_devices retains %d legacy secret/location columns", legacyColumns)
	}
}

func TestHelloUpdatesCapabilitiesAndDisabledNodeIsRejected(t *testing.T) {
	store, _ := newTestStore(t)
	pairing, err := store.CreatePairingCode(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	node, err := store.Pair(t.Context(), PairInput{Code: pairing.Code, DeviceID: "device_12345678", Name: "DockMini"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateHello(t.Context(), node.ID, Hello{
		DeviceID: "device_12345678", Version: "0.8.0", ProtocolVersion: ConnectionProtocolVersion,
		OS: "linux", Arch: "amd64", Capabilities: []string{"read_file", "read_file", "exec_command"}, ToolContractHash: "sha256:test",
		Tools: []ToolDescriptor{{Name: "read_file", InputSchema: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Capabilities) != 2 || updated.LastSeenAt == nil {
		t.Fatalf("unexpected hello state: %#v", updated)
	}
	descriptors, err := store.ToolDescriptors(t.Context(), node.ID)
	if err != nil || len(descriptors) != 1 || descriptors[0].Name != "read_file" {
		t.Fatalf("tool descriptors = %#v err=%v", descriptors, err)
	}
	enabled := false
	if _, err := store.Update(t.Context(), node.ID, UpdateInput{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateHello(t.Context(), node.ID, Hello{DeviceID: node.DeviceID}); !errors.Is(err, ErrNodeDisabled) {
		t.Fatalf("disabled hello error = %v", err)
	}
}
