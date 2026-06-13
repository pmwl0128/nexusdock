package artifacts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFetchLifecycleUsesHighRiskCommandAndDualAuthorization(t *testing.T) {
	fixture := newArtifactTestFixture(t)
	defer fixture.close()
	ctx := context.Background()
	receiver := make([]byte, 32)
	_, _ = rand.Read(receiver)
	created, err := fixture.service.CreateFetch(ctx, fixture.deviceID, CreateFetchRequest{
		SourceDeviceID: fixture.deviceID, SourcePath: "/tmp/fetch-source.txt",
		ReceiverPublicKey: base64.RawURLEncoding.EncodeToString(receiver), RetentionSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("CreateFetch: %v", err)
	}
	if created.Fetch.Status != FetchQueued || created.DownloadToken == "" {
		t.Fatalf("unexpected create result %#v", created)
	}
	commands, err := fixture.commands.ListByDevice(ctx, fixture.deviceID)
	if err != nil || len(commands) != 1 {
		t.Fatalf("commands=%#v err=%v", commands, err)
	}
	if commands[0].Type != "artifact.fetch" || commands[0].Risk != "high" {
		t.Fatalf("unexpected fetch command %#v", commands[0])
	}
	var payload map[string]any
	if err := json.Unmarshal(commands[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	uploadToken, _ := payload["upload_token"].(string)
	if uploadToken == "" {
		t.Fatal("fetch command omitted upload token")
	}
	if _, err := fixture.service.GetFetch(ctx, fixture.deviceID, created.Fetch.ID, "wrong"); err == nil {
		t.Fatal("wrong requester token accepted")
	}
	if _, err := fixture.service.BeginFetchUpload(ctx, "wrong-device", created.Fetch.ID, uploadToken); err == nil {
		t.Fatal("wrong source device accepted")
	}
	lease, err := fixture.service.BeginFetchUpload(ctx, fixture.deviceID, created.Fetch.ID, uploadToken)
	if err != nil {
		t.Fatalf("BeginFetchUpload: %v", err)
	}
	ciphertext := []byte("opaque fetch ciphertext")
	if err := os.WriteFile(lease.TempPath, ciphertext, 0o600); err != nil {
		t.Fatal(err)
	}
	plain := sha256.Sum256([]byte("plain fetch"))
	cipher := sha256.Sum256(ciphertext)
	ephemeral, wrapped, nonce := make([]byte, 32), make([]byte, 48), make([]byte, 12)
	_, _ = rand.Read(ephemeral)
	_, _ = rand.Read(wrapped)
	_, _ = rand.Read(nonce)
	ready, err := fixture.service.CompleteFetchUpload(ctx, lease, FetchManifest{
		FormatVersion: "ADR1", CipherAlgorithm: "AES-256-GCM-CHUNKED", Filename: "fetch.txt",
		ContentType: "text/plain", PlainSize: 11, PlainSHA256: hex.EncodeToString(plain[:]),
		EphemeralPublicKey: base64.RawURLEncoding.EncodeToString(ephemeral),
		WrappedKey:         base64.RawURLEncoding.EncodeToString(wrapped), WrapNonce: base64.RawURLEncoding.EncodeToString(nonce),
	}, int64(len(ciphertext)), hex.EncodeToString(cipher[:]))
	if err != nil || ready.Status != FetchReady {
		t.Fatalf("CompleteFetchUpload=%#v err=%v", ready, err)
	}
	grant, err := fixture.service.AuthorizeFetchDownload(ctx, fixture.deviceID, created.Fetch.ID, created.DownloadToken)
	if err != nil {
		t.Fatalf("AuthorizeFetchDownload: %v", err)
	}
	if actual, _ := os.ReadFile(grant.Path); string(actual) != string(ciphertext) {
		t.Fatalf("ciphertext mismatch %q", actual)
	}
	mounted, err := fixture.service.ConfirmFetchMounted(ctx, fixture.deviceID, created.Fetch.ID, created.DownloadToken)
	if err != nil || mounted.Status != FetchMounted {
		t.Fatalf("ConfirmFetchMounted=%#v err=%v", mounted, err)
	}
	if _, err := os.Stat(grant.Path); !os.IsNotExist(err) {
		t.Fatalf("ciphertext still exists: %v", err)
	}
}

func TestFetchDirectoryListingResult(t *testing.T) {
	fixture := newArtifactTestFixture(t)
	defer fixture.close()
	ctx := context.Background()
	public := make([]byte, 32)
	_, _ = rand.Read(public)
	created, err := fixture.service.CreateFetch(ctx, fixture.deviceID, CreateFetchRequest{
		SourceDeviceID: fixture.deviceID, SourcePath: "/tmp/directory", ReceiverPublicKey: base64.RawURLEncoding.EncodeToString(public), RetentionSeconds: 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	commands, _ := fixture.commands.ListByDevice(ctx, fixture.deviceID)
	var payload map[string]any
	_ = json.Unmarshal(commands[0].Payload, &payload)
	token, _ := payload["upload_token"].(string)
	listed, err := fixture.service.ReportFetchResult(ctx, fixture.deviceID, created.Fetch.ID, token, FetchResultRequest{
		Status: FetchListed, Listing: []FetchEntry{{Name: "a.txt", Path: "/tmp/directory/a.txt", Type: "file", Size: 5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Status != FetchListed || len(listed.Listing) != 1 || !strings.HasSuffix(listed.Listing[0].Path, "a.txt") {
		t.Fatalf("unexpected listing %#v", listed)
	}
}
