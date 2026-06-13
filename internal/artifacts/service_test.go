package artifacts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/core"
	"github.com/uvwt/agentdock-nexus/internal/devices"
)

type artifactTestFixture struct {
	service     *Service
	commands    *commands.Service
	deviceID    string
	deviceToken string
	publicKey   string
	root        string
	close       func()
}

func newArtifactTestFixture(t *testing.T) artifactTestFixture {
	t.Helper()
	ctx := context.Background()
	db, err := core.OpenSQLite(ctx, filepath.Join(t.TempDir(), "nexus.db"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.NewMigrationRunner(db, nil).Run(ctx); err != nil {
		db.Close()
		t.Fatal(err)
	}
	deviceService, err := devices.NewService(devices.NewSQLiteRepository(db))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	policy := devices.Policy{AllowedCommandTypes: []string{"artifact.pull", "artifact.fetch"}, MaxRisk: devices.RiskHigh, ReleaseChannel: devices.ChannelStable}
	token, err := deviceService.CreateEnrollmentToken(ctx, "test", time.Hour, policy)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	enrolled, err := deviceService.Enroll(ctx, devices.EnrollmentRequest{Token: token.PlainToken, Name: "DockMini", Platform: "darwin", Arch: "arm64", AgentDockVersion: "test", PublicKey: "ed25519:test"})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := deviceService.Approve(ctx, enrolled.Device.ID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		db.Close()
		t.Fatal(err)
	}
	publicKey := base64.RawURLEncoding.EncodeToString(key)
	_, err = deviceService.Heartbeat(ctx, enrolled.Device.ID, enrolled.DeviceToken, devices.Heartbeat{
		DeviceID: enrolled.Device.ID, SentAt: time.Now().UTC(), UptimeSeconds: 10, AgentDockVersion: "test",
		Capabilities: []devices.Capability{{Name: "artifact-relay", Version: "ADR1", Enabled: true, Metadata: map[string]string{"x25519_public_key": publicKey, "fetch_enabled": "true"}}},
	})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	commandService, err := commands.NewService(commands.NewSQLiteRepository(db), deviceService)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "artifacts")
	service, err := NewService(NewSQLiteRepository(db), deviceService, commandService, root)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	return artifactTestFixture{service: service, commands: commandService, deviceID: enrolled.Device.ID, deviceToken: enrolled.DeviceToken, publicKey: publicKey, root: root, close: func() { _ = db.Close() }}
}

func TestUploadTokenManifestDispatchAndDownloadAuthorization(t *testing.T) {
	fixture := newArtifactTestFixture(t)
	defer fixture.close()
	ctx := context.Background()
	dispatch := true
	created, err := fixture.service.CreateUpload(ctx, "device", fixture.deviceID, CreateUploadRequest{
		Filename: "report.bin", TargetDeviceIDs: []string{fixture.deviceID}, Dispatch: &dispatch,
		ConflictPolicy: "reject", LogicalTarget: "inbox",
	})
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	if len(created.Targets) != 1 || created.Targets[0].X25519PublicKey != fixture.publicKey {
		t.Fatalf("unexpected targets %#v", created.Targets)
	}
	if _, err := fixture.service.BeginUpload(ctx, created.Artifact.ID, "wrong"); err == nil {
		t.Fatal("wrong upload token unexpectedly accepted")
	}
	lease, err := fixture.service.BeginUpload(ctx, created.Artifact.ID, created.UploadToken)
	if err != nil {
		t.Fatalf("BeginUpload: %v", err)
	}
	ciphertext := []byte("opaque encrypted payload")
	if err := os.WriteFile(lease.TempPath, ciphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	cipherHash := sha256.Sum256(ciphertext)
	plainHash := sha256.Sum256([]byte("plain"))
	ephemeral := make([]byte, 32)
	wrapped := make([]byte, 48)
	nonce := make([]byte, 12)
	_, _ = rand.Read(ephemeral)
	_, _ = rand.Read(wrapped)
	_, _ = rand.Read(nonce)
	completed, err := fixture.service.CompleteUpload(ctx, lease, UploadManifest{
		FormatVersion: "ADR1", CipherAlgorithm: "AES-256-GCM-CHUNKED", PlainSize: 5, PlainSHA256: hex.EncodeToString(plainHash[:]),
		EphemeralPublicKey: base64.RawURLEncoding.EncodeToString(ephemeral),
		WrappedKeys:        []WrappedKeyManifest{{DeliveryID: created.Deliveries[0].ID, TargetDeviceID: fixture.deviceID, WrappedKey: base64.RawURLEncoding.EncodeToString(wrapped), WrapNonce: base64.RawURLEncoding.EncodeToString(nonce)}},
	}, int64(len(ciphertext)), hex.EncodeToString(cipherHash[:]))
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if completed.Artifact.Status != ArtifactUploaded || completed.Deliveries[0].Status != DeliveryQueued {
		t.Fatalf("unexpected completion %#v", completed)
	}
	if _, err := fixture.service.BeginUpload(ctx, created.Artifact.ID, created.UploadToken); err == nil {
		t.Fatal("one-time upload token was reused")
	}
	items, err := fixture.commands.ListByDevice(ctx, fixture.deviceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("commands: %#v err=%v", items, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(items[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	deliveryToken, _ := payload["download_token"].(string)
	if deliveryToken == "" {
		t.Fatal("dispatch omitted delivery token")
	}
	if _, err := fixture.service.AuthorizeDownload(ctx, "other-device", created.Deliveries[0].ID, deliveryToken); err == nil {
		t.Fatal("wrong device unexpectedly authorized")
	}
	if _, err := fixture.service.AuthorizeDownload(ctx, fixture.deviceID, created.Deliveries[0].ID, "wrong"); err == nil {
		t.Fatal("wrong delivery token unexpectedly authorized")
	}
	grant, err := fixture.service.AuthorizeDownload(ctx, fixture.deviceID, created.Deliveries[0].ID, deliveryToken)
	if err != nil {
		t.Fatalf("AuthorizeDownload: %v", err)
	}
	actual, err := os.ReadFile(grant.Path)
	if err != nil || string(actual) != string(ciphertext) {
		t.Fatalf("ciphertext mismatch %q err=%v", actual, err)
	}
	result, err := fixture.service.ReportDelivery(ctx, fixture.deviceID, created.Deliveries[0].ID, deliveryToken, DeliveryResultRequest{Status: DeliveryCompleted, LocalPath: "/inbox/report.bin"})
	if err != nil || result.Status != DeliveryCompleted {
		t.Fatalf("ReportDelivery: %#v err=%v", result, err)
	}
}

func TestUploadRejectsOversizeAndInvalidManifest(t *testing.T) {
	fixture := newArtifactTestFixture(t)
	defer fixture.close()
	ctx := context.Background()
	dispatch := false
	created, err := fixture.service.CreateUpload(ctx, "api", "", CreateUploadRequest{Filename: "x", TargetDeviceIDs: []string{fixture.deviceID}, Dispatch: &dispatch})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := fixture.service.BeginUpload(ctx, created.Artifact.ID, created.UploadToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lease.TempPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.service.CompleteUpload(ctx, lease, UploadManifest{}, DefaultMaxCipherBytes+1, strings.Repeat("0", 64))
	var artifactErr *Error
	if !errors.As(err, &artifactErr) || artifactErr.Code != ErrTooLarge {
		t.Fatalf("expected too large error, got %v", err)
	}
}
