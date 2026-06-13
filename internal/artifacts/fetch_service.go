package artifacts

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/core"
	"github.com/uvwt/agentdock-nexus/internal/devices"
)

type FetchUploadLease struct {
	Fetch    FetchJob
	TempPath string
	Path     string
}

func (s *Service) CreateFetch(ctx context.Context, requesterDeviceID string, request CreateFetchRequest) (CreateFetchResult, error) {
	now := s.clock.Now().UTC()
	requesterDeviceID = strings.TrimSpace(requesterDeviceID)
	request.SourceDeviceID = strings.TrimSpace(request.SourceDeviceID)
	request.SourcePath = strings.TrimSpace(request.SourcePath)
	if requesterDeviceID == "" || request.SourceDeviceID == "" {
		return CreateFetchResult{}, domainError(ErrValidation, "requester and source device ids are required")
	}
	if request.SourcePath == "" || len(request.SourcePath) > 4096 || !filepath.IsAbs(request.SourcePath) {
		return CreateFetchResult{}, domainError(ErrValidation, "source_path must be an absolute path")
	}
	if value, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(request.ReceiverPublicKey)); err != nil || len(value) != 32 {
		return CreateFetchResult{}, domainError(ErrValidation, "receiver X25519 public key is invalid")
	}
	if _, err := s.devices.Snapshot(ctx, requesterDeviceID); err != nil {
		return CreateFetchResult{}, err
	}
	source, err := s.devices.Snapshot(ctx, request.SourceDeviceID)
	if err != nil {
		return CreateFetchResult{}, err
	}
	if _, err := artifactPublicKey(source); err != nil {
		return CreateFetchResult{}, err
	}
	retention := DefaultRetention
	if request.RetentionSeconds != 0 {
		retention = time.Duration(request.RetentionSeconds) * time.Second
	}
	if retention < MinimumRetention || retention > MaximumRetention {
		return CreateFetchResult{}, domainError(ErrValidation, "retention must be between 1 hour and 7 days")
	}
	fetchID, err := core.NewID("fet")
	if err != nil {
		return CreateFetchResult{}, err
	}
	uploadToken, err := randomToken(32)
	if err != nil {
		return CreateFetchResult{}, err
	}
	downloadToken, err := randomToken(32)
	if err != nil {
		return CreateFetchResult{}, err
	}
	expiresAt := now.Add(retention)
	job := FetchJob{
		ID: fetchID, RequesterDeviceID: requesterDeviceID, SourceDeviceID: request.SourceDeviceID,
		SourcePath: request.SourcePath, ArchiveRequested: request.Archive, Status: FetchPending,
		StoragePath:       filepath.ToSlash(filepath.Join("fetches", fetchID, "payload.adr")),
		ReceiverPublicKey: strings.TrimSpace(request.ReceiverPublicKey),
		UploadTokenDigest: digestToken(uploadToken), UploadTokenExpiresAt: expiresAt,
		DownloadTokenDigest: digestToken(downloadToken), DownloadTokenExpiresAt: expiresAt,
		ExpiresAt: expiresAt, CreatedAt: now, UpdatedAt: now, Listing: []FetchEntry{},
	}
	if err := s.fetches.CreateFetch(ctx, job); err != nil {
		return CreateFetchResult{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"fetch_id": fetchID, "requester_device_id": requesterDeviceID, "source_path": request.SourcePath,
		"archive": request.Archive, "receiver_public_key": job.ReceiverPublicKey,
		"upload_token": uploadToken,
		"upload_path":  "/v1/devices/" + request.SourceDeviceID + "/artifact-fetches/" + fetchID + "/content",
		"result_path":  "/v1/devices/" + request.SourceDeviceID + "/artifact-fetches/" + fetchID + "/result",
		"expires_at":   expiresAt.Format(time.RFC3339Nano), "max_cipher_bytes": s.maxBytes,
	})
	if err != nil {
		return CreateFetchResult{}, err
	}
	command, _, err := s.commands.Enqueue(ctx, commands.EnqueueRequest{
		DeviceID: request.SourceDeviceID, Type: commands.TypeArtifactFetch, Risk: devices.RiskHigh, Payload: payload,
		IdempotencyKey: "artifact-fetch:" + fetchID, Priority: 10, MaxAttempts: 3,
		NotBefore: now, ExpiresAt: expiresAt, CreatedBy: "artifact-fetch",
	})
	if err != nil {
		return CreateFetchResult{}, err
	}
	job, err = s.fetches.SetFetchCommand(ctx, fetchID, command.ID, FetchQueued, now)
	if err != nil {
		return CreateFetchResult{}, err
	}
	return CreateFetchResult{Fetch: publicFetch(job), DownloadToken: downloadToken}, nil
}

func (s *Service) GetFetch(ctx context.Context, requesterDeviceID, fetchID, token string) (FetchJob, error) {
	job, err := s.fetches.GetFetch(ctx, strings.TrimSpace(fetchID))
	if err != nil {
		return FetchJob{}, err
	}
	if err := authorizeFetchRequester(job, requesterDeviceID, token, s.clock.Now().UTC()); err != nil {
		return FetchJob{}, err
	}
	return publicFetch(job), nil
}

func (s *Service) BeginFetchUpload(ctx context.Context, sourceDeviceID, fetchID, token string) (FetchUploadLease, error) {
	now := s.clock.Now().UTC()
	job, err := s.fetches.ClaimFetchUpload(ctx, strings.TrimSpace(fetchID), strings.TrimSpace(sourceDeviceID), digestToken(token), now)
	if err != nil {
		return FetchUploadLease{}, err
	}
	path, err := s.storagePath(job.StoragePath)
	if err != nil {
		_ = s.fetches.AbortFetchUpload(ctx, job.ID, now)
		return FetchUploadLease{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		_ = s.fetches.AbortFetchUpload(ctx, job.ID, now)
		return FetchUploadLease{}, fmt.Errorf("create fetch directory: %w", err)
	}
	temp := filepath.Join(filepath.Dir(path), "upload.part")
	_ = os.Remove(temp)
	return FetchUploadLease{Fetch: job, TempPath: temp, Path: path}, nil
}

func (s *Service) AbortFetchUpload(ctx context.Context, lease FetchUploadLease) {
	_ = os.Remove(lease.TempPath)
	_ = s.fetches.AbortFetchUpload(ctx, lease.Fetch.ID, s.clock.Now().UTC())
}

func (s *Service) CompleteFetchUpload(ctx context.Context, lease FetchUploadLease, manifest FetchManifest, cipherSize int64, cipherSHA256 string) (FetchJob, error) {
	if cipherSize <= 0 || cipherSize > s.maxBytes {
		s.AbortFetchUpload(ctx, lease)
		return FetchJob{}, domainError(ErrTooLarge, "encrypted fetch exceeds %d bytes", s.maxBytes)
	}
	if manifest.FormatVersion != "ADR1" || manifest.CipherAlgorithm != "AES-256-GCM-CHUNKED" {
		s.AbortFetchUpload(ctx, lease)
		return FetchJob{}, domainError(ErrValidation, "unsupported fetch encryption format")
	}
	filename, err := validateFilename(manifest.Filename)
	if err != nil {
		s.AbortFetchUpload(ctx, lease)
		return FetchJob{}, err
	}
	if manifest.PlainSize < 0 || !validSHA256(manifest.PlainSHA256) || !validBase64Size(manifest.EphemeralPublicKey, 32) || !validBase64Size(manifest.WrappedKey, 48) || !validBase64Size(manifest.WrapNonce, 12) {
		s.AbortFetchUpload(ctx, lease)
		return FetchJob{}, domainError(ErrValidation, "fetch manifest cryptographic metadata is invalid")
	}
	if err := os.Chmod(lease.TempPath, 0o600); err != nil {
		s.AbortFetchUpload(ctx, lease)
		return FetchJob{}, err
	}
	if err := os.Rename(lease.TempPath, lease.Path); err != nil {
		s.AbortFetchUpload(ctx, lease)
		return FetchJob{}, fmt.Errorf("publish fetch upload: %w", err)
	}
	now := s.clock.Now().UTC()
	used := now
	job := lease.Fetch
	job.Status = FetchReady
	job.Filename = filename
	job.ContentType = strings.TrimSpace(manifest.ContentType)
	if job.ContentType == "" {
		job.ContentType = "application/octet-stream"
	}
	job.EphemeralPublicKey = manifest.EphemeralPublicKey
	job.WrappedKey = manifest.WrappedKey
	job.WrapNonce = manifest.WrapNonce
	job.PlainSize = manifest.PlainSize
	job.PlainSHA256 = strings.ToLower(manifest.PlainSHA256)
	job.CipherSize = cipherSize
	job.CipherSHA256 = strings.ToLower(cipherSHA256)
	job.UploadTokenUsedAt = &used
	job.UpdatedAt = now
	if err := s.fetches.CompleteFetchUpload(ctx, job); err != nil {
		_ = os.Remove(lease.Path)
		_ = s.fetches.AbortFetchUpload(ctx, job.ID, now)
		return FetchJob{}, err
	}
	return publicFetch(job), nil
}

func (s *Service) ReportFetchResult(ctx context.Context, sourceDeviceID, fetchID, token string, request FetchResultRequest) (FetchJob, error) {
	job, err := s.fetches.GetFetch(ctx, strings.TrimSpace(fetchID))
	if err != nil {
		return FetchJob{}, err
	}
	now := s.clock.Now().UTC()
	if job.SourceDeviceID != strings.TrimSpace(sourceDeviceID) {
		return FetchJob{}, domainError(ErrFetchDeviceMismatch, "fetch does not belong to source device")
	}
	if err := validateDigestToken(job.UploadTokenDigest, token, job.UploadTokenExpiresAt, now); err != nil {
		return FetchJob{}, err
	}
	if request.Status != FetchListed && request.Status != FetchFailed {
		return FetchJob{}, domainError(ErrValidation, "fetch result status must be listed or failed")
	}
	if len(request.Listing) > 1000 || len(request.ErrorCode) > 128 || len(request.ErrorMessage) > 4096 {
		return FetchJob{}, domainError(ErrValidation, "fetch result exceeds limits")
	}
	for _, entry := range request.Listing {
		if entry.Name == "" || entry.Path == "" || len(entry.Path) > 4096 || (entry.Type != "file" && entry.Type != "directory") || entry.Size < 0 {
			return FetchJob{}, domainError(ErrValidation, "fetch listing entry is invalid")
		}
	}
	updated, err := s.fetches.SetFetchResult(ctx, job.ID, sourceDeviceID, request, now)
	if err != nil {
		return FetchJob{}, err
	}
	return publicFetch(updated), nil
}

func (s *Service) AuthorizeFetchDownload(ctx context.Context, requesterDeviceID, fetchID, token string) (FetchDownloadGrant, error) {
	job, err := s.fetches.GetFetch(ctx, strings.TrimSpace(fetchID))
	if err != nil {
		return FetchDownloadGrant{}, err
	}
	now := s.clock.Now().UTC()
	if err := authorizeFetchRequester(job, requesterDeviceID, token, now); err != nil {
		return FetchDownloadGrant{}, err
	}
	if job.Status != FetchReady {
		return FetchDownloadGrant{}, domainError(ErrInvalidState, "fetch is not ready")
	}
	path, err := s.storagePath(job.StoragePath)
	if err != nil {
		return FetchDownloadGrant{}, err
	}
	if _, err := os.Stat(path); err != nil {
		return FetchDownloadGrant{}, domainError(ErrFetchNotFound, "fetch payload is missing")
	}
	return FetchDownloadGrant{Fetch: publicFetch(job), Path: path}, nil
}

func (s *Service) ConfirmFetchMounted(ctx context.Context, requesterDeviceID, fetchID, token string) (FetchJob, error) {
	job, err := s.fetches.GetFetch(ctx, strings.TrimSpace(fetchID))
	if err != nil {
		return FetchJob{}, err
	}
	now := s.clock.Now().UTC()
	if err := authorizeFetchRequester(job, requesterDeviceID, token, now); err != nil {
		return FetchJob{}, err
	}
	if job.Status != FetchReady {
		return FetchJob{}, domainError(ErrInvalidState, "fetch is not ready for mounted confirmation")
	}
	path, err := s.storagePath(job.StoragePath)
	if err != nil {
		return FetchJob{}, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return FetchJob{}, fmt.Errorf("delete fetch ciphertext: %w", err)
	}
	_ = os.Remove(filepath.Dir(path))
	updated, err := s.fetches.MarkFetchMounted(ctx, job.ID, requesterDeviceID, now)
	if err != nil {
		return FetchJob{}, err
	}
	return publicFetch(updated), nil
}

func (s *Service) cleanupExpiredFetches(ctx context.Context) (int, error) {
	items, err := s.fetches.ExpireFetches(ctx, s.clock.Now().UTC())
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		if path, pathErr := s.storagePath(item.StoragePath); pathErr == nil {
			_ = os.Remove(path)
			_ = os.Remove(filepath.Dir(path))
		}
	}
	return len(items), nil
}

func artifactFetchCapabilityEnabled(snapshot devices.Snapshot) bool {
	capabilities := snapshot.Device.Capabilities
	if snapshot.Heartbeat != nil {
		capabilities = snapshot.Heartbeat.Capabilities
	}
	for _, capability := range capabilities {
		if capability.Name == "artifact-relay" && capability.Enabled && strings.EqualFold(capability.Metadata["fetch_enabled"], "true") {
			return true
		}
	}
	return false
}

func authorizeFetchRequester(job FetchJob, requesterDeviceID, token string, now time.Time) error {
	if job.RequesterDeviceID != strings.TrimSpace(requesterDeviceID) {
		return domainError(ErrFetchDeviceMismatch, "fetch does not belong to requester device")
	}
	return validateDigestToken(job.DownloadTokenDigest, token, job.DownloadTokenExpiresAt, now)
}

func validateDigestToken(expected, token string, expiresAt, now time.Time) error {
	if strings.TrimSpace(token) == "" {
		return domainError(ErrFetchTokenInvalid, "fetch token is required")
	}
	actual := digestToken(token)
	if len(actual) != len(expected) || subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		return domainError(ErrFetchTokenInvalid, "fetch token is invalid")
	}
	if !now.Before(expiresAt) {
		return domainError(ErrFetchTokenExpired, "fetch token expired")
	}
	return nil
}

func validBase64Size(value string, size int) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == size
}

func publicFetch(job FetchJob) FetchJob {
	job.ReceiverPublicKey = ""
	job.WrappedKey = ""
	job.WrapNonce = ""
	job.UploadTokenDigest = ""
	job.DownloadTokenDigest = ""
	job.UploadTokenExpiresAt = time.Time{}
	job.DownloadTokenExpiresAt = time.Time{}
	job.UploadTokenUsedAt = nil
	job.StoragePath = ""
	return job
}

func fetchCipherHash(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := file.WriteTo(hash)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}
