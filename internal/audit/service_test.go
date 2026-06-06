package audit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/memorydock/internal/core"
)

func TestAuditEventsAreAppendOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := core.OpenSQLite(ctx, filepath.Join(t.TempDir(), "nexus.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := core.NewMigrationRunner(db, nil).Run(ctx); err != nil {
		t.Fatal(err)
	}
	service := NewService(db)
	event, err := service.Record(ctx, Event{
		Actor:  core.Actor{Type: core.ActorSystem, ID: "test"},
		Action: "object.create", ObjectType: "object", ObjectID: "o1", Result: "succeeded",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE audit_events SET result = 'changed' WHERE id = ?`, event.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("update error = %v, want append-only rejection", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM audit_events WHERE id = ?`, event.ID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("delete error = %v, want append-only rejection", err)
	}
	events, err := service.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("events = %#v", events)
	}
}
