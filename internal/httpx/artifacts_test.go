package httpx

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/artifacts"
	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/config"
	"github.com/uvwt/agentdock-nexus/internal/core"
	"github.com/uvwt/agentdock-nexus/internal/devices"
	"github.com/uvwt/agentdock-nexus/internal/memory"
	"github.com/uvwt/agentdock-nexus/internal/syncer"
)

type artifactHTTPFixture struct {
	handler     http.Handler
	commands    *commands.Service
	deviceID    string
	deviceToken string
	close       func()
}

func newArtifactHTTPFixture(t *testing.T) artifactHTTPFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	store, err := memory.NewStore(filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	mgr := syncer.NewManager(syncer.Config{RepoDir: store.Root()}, slog.Default())
	db, err := core.OpenSQLite(ctx, filepath.Join(root, "control.db"), 4)
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
	publicKey := make([]byte, 32)
	_, _ = rand.Read(publicKey)
	_, err = deviceService.Heartbeat(ctx, enrolled.Device.ID, enrolled.DeviceToken, devices.Heartbeat{
		DeviceID: enrolled.Device.ID, SentAt: time.Now().UTC(), UptimeSeconds: 1, AgentDockVersion: "test",
		Capabilities: []devices.Capability{{Name: "artifact-relay", Version: "ADR1", Enabled: true, Metadata: map[string]string{"x25519_public_key": base64.RawURLEncoding.EncodeToString(publicKey), "fetch_enabled": "true"}}},
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
	artifactService, err := artifacts.NewService(artifacts.NewSQLiteRepository(db), deviceService, commandService, filepath.Join(root, "artifacts"))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	handler := NewServer(config.Config{StoreDir: store.Root(), AuthToken: "admin-token"}, store, mgr, slog.Default(), WithSystemDatabase(db), WithControlPlane(deviceService, commandService), WithArtifactRelay(artifactService)).Handler()
	return artifactHTTPFixture{handler: handler, commands: commandService, deviceID: enrolled.Device.ID, deviceToken: enrolled.DeviceToken, close: func() { _ = db.Close() }}
}

func TestArtifactHTTPUploadAndDualAuthenticatedDownload(t *testing.T) {
	fixture := newArtifactHTTPFixture(t)
	defer fixture.close()
	unauthorized := controlPlaneRequest(t, fixture.handler, http.MethodPost, "/v1/artifacts/uploads", "", artifacts.CreateUploadRequest{Filename: "report.bin", TargetDeviceIDs: []string{fixture.deviceID}})
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized create status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
	dispatch := true
	createdResponse := controlPlaneRequest(t, fixture.handler, http.MethodPost, "/v1/artifacts/uploads", "admin-token", artifacts.CreateUploadRequest{
		Filename: "report.bin", TargetDeviceIDs: []string{fixture.deviceID}, Dispatch: &dispatch, ConflictPolicy: "reject", LogicalTarget: "inbox",
	})
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	created := decodeResponse[artifacts.CreateUploadResult](t, createdResponse)
	ciphertext := []byte("ciphertext-only-on-nexus")
	plainHash := sha256.Sum256([]byte("plain"))
	ephemeral := randomBytes(t, 32)
	wrapped := randomBytes(t, 48)
	nonce := randomBytes(t, 12)
	manifest := artifacts.UploadManifest{
		FormatVersion: "ADR1", CipherAlgorithm: "AES-256-GCM-CHUNKED", PlainSize: 5, PlainSHA256: hex.EncodeToString(plainHash[:]),
		EphemeralPublicKey: base64.RawURLEncoding.EncodeToString(ephemeral),
		WrappedKeys:        []artifacts.WrappedKeyManifest{{DeliveryID: created.Deliveries[0].ID, TargetDeviceID: fixture.deviceID, WrappedKey: base64.RawURLEncoding.EncodeToString(wrapped), WrapNonce: base64.RawURLEncoding.EncodeToString(nonce)}},
	}
	upload := artifactMultipartRequest(t, fixture.handler, created.UploadPath, created.UploadToken, manifest, ciphertext)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", upload.Code, upload.Body.String())
	}
	completion := decodeResponse[artifacts.UploadCompletion](t, upload)
	if completion.Artifact.Status != artifacts.ArtifactUploaded || completion.Deliveries[0].Status != artifacts.DeliveryQueued {
		t.Fatalf("unexpected completion %#v", completion)
	}

	listedResponse := controlPlaneRequest(t, fixture.handler, http.MethodGet, "/v1/artifacts?limit=10", "admin-token", nil)
	if listedResponse.Code != http.StatusOK {
		t.Fatalf("list artifacts status=%d body=%s", listedResponse.Code, listedResponse.Body.String())
	}
	var listed struct {
		Items []artifacts.Detail `json:"items"`
	}
	if err := json.Unmarshal(listedResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Artifact.ID != created.Artifact.ID || len(listed.Items[0].Deliveries) != 1 {
		t.Fatalf("unexpected artifact list %#v", listed.Items)
	}
	reused := artifactMultipartRequest(t, fixture.handler, created.UploadPath, created.UploadToken, manifest, ciphertext)
	if reused.Code != http.StatusConflict {
		t.Fatalf("reused upload token status=%d body=%s", reused.Code, reused.Body.String())
	}

	commandItems, err := fixture.commands.ListByDevice(context.Background(), fixture.deviceID)
	if err != nil || len(commandItems) != 1 {
		t.Fatalf("commands=%#v err=%v", commandItems, err)
	}
	var commandPayload map[string]any
	if err := json.Unmarshal(commandItems[0].Payload, &commandPayload); err != nil {
		t.Fatal(err)
	}
	deliveryToken, _ := commandPayload["download_token"].(string)
	downloadPath, _ := commandPayload["download_path"].(string)
	resultPath, _ := commandPayload["result_path"].(string)
	if deliveryToken == "" || downloadPath == "" || resultPath == "" {
		t.Fatalf("incomplete command payload %#v", commandPayload)
	}

	noDeviceAuth := httptest.NewRequest(http.MethodGet, downloadPath, nil)
	noDeviceAuth.Header.Set("X-Artifact-Delivery-Token", deliveryToken)
	noDeviceResponse := httptest.NewRecorder()
	fixture.handler.ServeHTTP(noDeviceResponse, noDeviceAuth)
	if noDeviceResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing device auth status=%d", noDeviceResponse.Code)
	}
	wrongDelivery := httptest.NewRequest(http.MethodGet, downloadPath, nil)
	wrongDelivery.Header.Set("Authorization", "Bearer "+fixture.deviceToken)
	wrongDelivery.Header.Set("X-Artifact-Delivery-Token", "wrong")
	wrongDeliveryResponse := httptest.NewRecorder()
	fixture.handler.ServeHTTP(wrongDeliveryResponse, wrongDelivery)
	if wrongDeliveryResponse.Code != http.StatusUnauthorized {
		t.Fatalf("wrong delivery token status=%d body=%s", wrongDeliveryResponse.Code, wrongDeliveryResponse.Body.String())
	}
	valid := httptest.NewRequest(http.MethodGet, downloadPath, nil)
	valid.Header.Set("Authorization", "Bearer "+fixture.deviceToken)
	valid.Header.Set("X-Artifact-Delivery-Token", deliveryToken)
	validResponse := httptest.NewRecorder()
	fixture.handler.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK || !bytes.Equal(validResponse.Body.Bytes(), ciphertext) {
		t.Fatalf("download status=%d body=%q", validResponse.Code, validResponse.Body.Bytes())
	}

	resultBody, _ := json.Marshal(artifacts.DeliveryResultRequest{Status: artifacts.DeliveryCompleted, LocalPath: "/inbox/report.bin"})
	resultRequest := httptest.NewRequest(http.MethodPost, resultPath, bytes.NewReader(resultBody))
	resultRequest.Header.Set("Authorization", "Bearer "+fixture.deviceToken)
	resultRequest.Header.Set("X-Artifact-Delivery-Token", deliveryToken)
	resultRequest.Header.Set("Content-Type", "application/json")
	resultResponse := httptest.NewRecorder()
	fixture.handler.ServeHTTP(resultResponse, resultRequest)
	if resultResponse.Code != http.StatusOK {
		t.Fatalf("result status=%d body=%s", resultResponse.Code, resultResponse.Body.String())
	}
	status := controlPlaneRequest(t, fixture.handler, http.MethodGet, "/v1/artifacts/"+created.Artifact.ID, "admin-token", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}
	detail := decodeResponse[artifacts.UploadCompletion](t, status)
	if detail.Deliveries[0].Status != artifacts.DeliveryCompleted {
		t.Fatalf("delivery status=%s", detail.Deliveries[0].Status)
	}
}

func artifactMultipartRequest(t *testing.T, handler http.Handler, path, token string, manifest artifacts.UploadManifest, file []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	manifestPart, err := writer.CreateFormField("manifest")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(manifestPart).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	filePart, err := writer.CreateFormFile("file", "payload.adr")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := filePart.Write(file); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Artifact-Upload-Token", token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func randomBytes(t *testing.T, size int) []byte {
	t.Helper()
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestArtifactFetchHTTPAuthorization(t *testing.T) {
	fixture := newArtifactHTTPFixture(t)
	defer fixture.close()
	key := randomBytes(t, 32)
	body, _ := json.Marshal(artifacts.CreateFetchRequest{
		SourceDeviceID:    fixture.deviceID,
		SourcePath:        "/tmp/report.txt",
		ReceiverPublicKey: base64.RawURLEncoding.EncodeToString(key),
		RetentionSeconds:  3600,
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/devices/"+fixture.deviceID+"/artifact-fetches", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+fixture.deviceToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create=%d body=%s", response.Code, response.Body.String())
	}
	created := decodeResponse[artifacts.CreateFetchResult](t, response)

	listedResponse := controlPlaneRequest(t, fixture.handler, http.MethodGet, "/v1/artifact-fetches?limit=10", "admin-token", nil)
	if listedResponse.Code != http.StatusOK {
		t.Fatalf("list fetches status=%d body=%s", listedResponse.Code, listedResponse.Body.String())
	}
	var listed struct {
		Items []artifacts.FetchJob `json:"items"`
	}
	if err := json.Unmarshal(listedResponse.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != created.Fetch.ID {
		t.Fatalf("unexpected fetch list %#v", listed.Items)
	}

	missing := httptest.NewRequest(http.MethodGet, "/v1/devices/"+fixture.deviceID+"/artifact-fetches/"+created.Fetch.ID, nil)
	missing.Header.Set("Authorization", "Bearer "+fixture.deviceToken)
	missingResponse := httptest.NewRecorder()
	fixture.handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing fetch credential=%d", missingResponse.Code)
	}

	valid := httptest.NewRequest(http.MethodGet, "/v1/devices/"+fixture.deviceID+"/artifact-fetches/"+created.Fetch.ID, nil)
	valid.Header.Set("Authorization", "Bearer "+fixture.deviceToken)
	valid.Header.Set("X-Artifact-Fetch-Token", created.DownloadToken)
	validResponse := httptest.NewRecorder()
	fixture.handler.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
}

func TestPersonalControlPlaneSystemStatus(t *testing.T) {
	fixture := newArtifactHTTPFixture(t)
	defer fixture.close()
	response := controlPlaneRequest(t, fixture.handler, http.MethodGet, "/v1/system/status", "admin-token", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("system status=%d body=%s", response.Code, response.Body.String())
	}
	var status struct {
		OK            bool   `json:"ok"`
		Database      string `json:"database"`
		SchemaVersion int    `json:"schema_version"`
		RecallRoot    string `json:"recall_root"`
		ArtifactRoot  string `json:"artifact_root"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.OK || status.Database != "ok" || status.SchemaVersion == 0 || status.RecallRoot == "" || status.ArtifactRoot == "" {
		t.Fatalf("unexpected system status %#v", status)
	}
}
