package artifacts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/core"
	"github.com/uvwt/agentdock-nexus/internal/devices"
)

var logicalTargetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)

type DeviceService interface {
	Snapshot(context.Context, string) (devices.Snapshot, error)
}

type CommandService interface {
	Enqueue(context.Context, commands.EnqueueRequest) (commands.Command, bool, error)
}

type Clock interface{ Now() time.Time }
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	repository Repository
	fetches    FetchRepository
	devices    DeviceService
	commands   CommandService
	root       string
	maxBytes   int64
	clock      Clock
}

type UploadLease struct {
	Artifact Artifact
	TempPath string
	Path     string
}

type ServiceOption func(*Service)

func WithMaxBytes(value int64) ServiceOption { return func(s *Service) { s.maxBytes = value } }
func WithClock(clock Clock) ServiceOption    { return func(s *Service) { s.clock = clock } }

func NewService(repository Repository, deviceService DeviceService, commandService CommandService, root string, options ...ServiceOption) (*Service, error) {
	if repository == nil || deviceService == nil || commandService == nil {
		return nil, domainError(ErrValidation, "repository, devices, and commands are required")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || strings.TrimSpace(root) == "" {
		return nil, domainError(ErrValidation, "artifact root is invalid")
	}
	fetches, ok := repository.(FetchRepository)
	if !ok {
		return nil, domainError(ErrValidation, "repository does not support artifact fetch")
	}
	service := &Service{repository: repository, fetches: fetches, devices: deviceService, commands: commandService, root: absolute, maxBytes: DefaultMaxCipherBytes, clock: systemClock{}}
	for _, option := range options {
		option(service)
	}
	if service.maxBytes <= 0 || service.clock == nil {
		return nil, domainError(ErrValidation, "positive max bytes and clock are required")
	}
	if err := os.MkdirAll(service.root, 0o700); err != nil {
		return nil, fmt.Errorf("create artifact root: %w", err)
	}
	if err := os.Chmod(service.root, 0o700); err != nil {
		return nil, fmt.Errorf("secure artifact root: %w", err)
	}
	return service, nil
}

func (s *Service) MaxCipherBytes() int64 { return s.maxBytes }

func (s *Service) CreateUpload(ctx context.Context, sourceKind, sourceID string, request CreateUploadRequest) (CreateUploadResult, error) {
	now := s.clock.Now().UTC()
	filename, err := validateFilename(request.Filename)
	if err != nil {
		return CreateUploadResult{}, err
	}
	if len(request.TargetDeviceIDs) == 0 || len(request.TargetDeviceIDs) > 32 {
		return CreateUploadResult{}, domainError(ErrValidation, "target_device_ids must contain 1..32 devices")
	}
	retention := DefaultRetention
	if request.RetentionSeconds != 0 {
		retention = time.Duration(request.RetentionSeconds) * time.Second
	}
	if retention < MinimumRetention || retention > MaximumRetention {
		return CreateUploadResult{}, domainError(ErrValidation, "retention must be between 1 hour and 7 days")
	}
	conflict := strings.ToLower(strings.TrimSpace(request.ConflictPolicy))
	if conflict == "" {
		conflict = "reject"
	}
	if conflict != "reject" && conflict != "rename" && conflict != "overwrite" {
		return CreateUploadResult{}, domainError(ErrValidation, "conflict_policy must be reject, rename, or overwrite")
	}
	logicalTarget := strings.TrimSpace(request.LogicalTarget)
	if logicalTarget == "" {
		logicalTarget = "inbox"
	}
	if !logicalTargetPattern.MatchString(logicalTarget) {
		return CreateUploadResult{}, domainError(ErrValidation, "logical_target is invalid")
	}
	contentType := strings.TrimSpace(request.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if len(contentType) > 255 {
		return CreateUploadResult{}, domainError(ErrValidation, "content_type is too long")
	}
	dispatch := true
	if request.Dispatch != nil {
		dispatch = *request.Dispatch
	}
	artifactID, err := core.NewID("art")
	if err != nil {
		return CreateUploadResult{}, err
	}
	uploadToken, err := randomToken(32)
	if err != nil {
		return CreateUploadResult{}, err
	}
	artifact := Artifact{
		ID: artifactID, SourceKind: strings.TrimSpace(sourceKind), SourceID: strings.TrimSpace(sourceID), Filename: filename,
		ContentType: contentType, Status: ArtifactPending, StoragePath: filepath.ToSlash(filepath.Join(artifactID, "payload.adr")),
		UploadTokenDigest: digestToken(uploadToken), UploadTokenExpiresAt: now.Add(DefaultUploadTTL), ExpiresAt: now.Add(retention),
		DispatchRequested: dispatch, DeleteAfterAllDelivered: request.DeleteAfterAllDelivered, ConflictPolicy: conflict,
		ExtractRequested: request.Extract, LogicalTarget: logicalTarget, CreatedAt: now, UpdatedAt: now,
	}
	if artifact.SourceKind == "" {
		artifact.SourceKind = "api"
	}
	seen := map[string]struct{}{}
	deliveries := make([]Delivery, 0, len(request.TargetDeviceIDs))
	targets := make([]UploadTarget, 0, len(request.TargetDeviceIDs))
	for _, rawID := range request.TargetDeviceIDs {
		deviceID := strings.TrimSpace(rawID)
		if deviceID == "" {
			return CreateUploadResult{}, domainError(ErrValidation, "target device id cannot be empty")
		}
		if _, duplicate := seen[deviceID]; duplicate {
			return CreateUploadResult{}, domainError(ErrValidation, "duplicate target device %q", deviceID)
		}
		seen[deviceID] = struct{}{}
		snapshot, err := s.devices.Snapshot(ctx, deviceID)
		if err != nil {
			return CreateUploadResult{}, err
		}
		publicKey, err := artifactPublicKey(snapshot)
		if err != nil {
			return CreateUploadResult{}, err
		}
		deliveryID, err := core.NewID("del")
		if err != nil {
			return CreateUploadResult{}, err
		}
		delivery := Delivery{ID: deliveryID, ArtifactID: artifact.ID, TargetDeviceID: deviceID, Status: DeliveryPending, CreatedAt: now, UpdatedAt: now}
		deliveries = append(deliveries, delivery)
		targets = append(targets, UploadTarget{DeliveryID: deliveryID, TargetDeviceID: deviceID, X25519PublicKey: publicKey})
	}
	if err := s.repository.Create(ctx, artifact, deliveries); err != nil {
		return CreateUploadResult{}, err
	}
	return CreateUploadResult{Artifact: artifact, Deliveries: deliveries, Targets: targets, UploadToken: uploadToken, UploadPath: "/v1/artifacts/" + artifact.ID + "/content"}, nil
}

func (s *Service) BeginUpload(ctx context.Context, artifactID, uploadToken string) (UploadLease, error) {
	now := s.clock.Now().UTC()
	if strings.TrimSpace(uploadToken) == "" {
		return UploadLease{}, domainError(ErrUploadTokenInvalid, "upload token is required")
	}
	artifact, err := s.repository.ClaimUpload(ctx, strings.TrimSpace(artifactID), digestToken(uploadToken), now)
	if err != nil {
		return UploadLease{}, err
	}
	dir := filepath.Join(s.root, artifact.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		_ = s.repository.AbortUpload(ctx, artifact.ID, now)
		return UploadLease{}, fmt.Errorf("create artifact directory: %w", err)
	}
	tempPath := filepath.Join(dir, "upload.part")
	finalPath, err := s.storagePath(artifact.StoragePath)
	if err != nil {
		_ = s.repository.AbortUpload(ctx, artifact.ID, now)
		return UploadLease{}, err
	}
	_ = os.Remove(tempPath)
	return UploadLease{Artifact: artifact, TempPath: tempPath, Path: finalPath}, nil
}

func (s *Service) AbortUpload(ctx context.Context, lease UploadLease) {
	_ = os.Remove(lease.TempPath)
	_ = s.repository.AbortUpload(ctx, lease.Artifact.ID, s.clock.Now().UTC())
}

func (s *Service) CompleteUpload(ctx context.Context, lease UploadLease, manifest UploadManifest, cipherSize int64, cipherSHA256 string) (UploadCompletion, error) {
	if cipherSize <= 0 || cipherSize > s.maxBytes {
		s.AbortUpload(ctx, lease)
		return UploadCompletion{}, domainError(ErrTooLarge, "encrypted artifact exceeds %d bytes", s.maxBytes)
	}
	deliveries, err := s.repository.ListDeliveries(ctx, lease.Artifact.ID)
	if err != nil {
		s.AbortUpload(ctx, lease)
		return UploadCompletion{}, err
	}
	manifestDeliveries, err := validateManifest(manifest, deliveries)
	if err != nil {
		s.AbortUpload(ctx, lease)
		return UploadCompletion{}, err
	}
	if err := os.Chmod(lease.TempPath, 0o600); err != nil {
		s.AbortUpload(ctx, lease)
		return UploadCompletion{}, fmt.Errorf("secure artifact upload: %w", err)
	}
	if err := os.Rename(lease.TempPath, lease.Path); err != nil {
		s.AbortUpload(ctx, lease)
		return UploadCompletion{}, fmt.Errorf("publish artifact upload: %w", err)
	}
	now := s.clock.Now().UTC()
	used := now
	artifact := lease.Artifact
	artifact.Status = ArtifactUploaded
	artifact.CipherSize = cipherSize
	artifact.CipherSHA256 = strings.ToLower(cipherSHA256)
	artifact.PlainSize = manifest.PlainSize
	artifact.PlainSHA256 = strings.ToLower(manifest.PlainSHA256)
	artifact.EphemeralPublicKey = manifest.EphemeralPublicKey
	artifact.UploadTokenUsedAt = &used
	artifact.UpdatedAt = now
	for i := range manifestDeliveries {
		manifestDeliveries[i].UpdatedAt = now
	}
	if err := s.repository.FinalizeUpload(ctx, artifact, manifestDeliveries); err != nil {
		_ = os.Remove(lease.Path)
		_ = s.repository.AbortUpload(ctx, artifact.ID, now)
		return UploadCompletion{}, err
	}
	if artifact.DispatchRequested {
		// A dispatch failure is recorded per Delivery. The encrypted upload remains
		// durable and can be retried explicitly without uploading the file again.
		_, _ = s.Dispatch(ctx, artifact.ID)
	}
	return s.Get(ctx, artifact.ID)
}

func (s *Service) Dispatch(ctx context.Context, artifactID string) ([]Delivery, error) {
	artifact, err := s.repository.GetArtifact(ctx, strings.TrimSpace(artifactID))
	if err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	if artifact.Status != ArtifactUploaded || !now.Before(artifact.ExpiresAt) {
		return nil, domainError(ErrInvalidState, "artifact is not available for dispatch")
	}
	deliveries, err := s.repository.ListDeliveries(ctx, artifact.ID)
	if err != nil {
		return nil, err
	}
	result := make([]Delivery, 0, len(deliveries))
	for _, delivery := range deliveries {
		if delivery.Status == DeliveryCompleted || delivery.Status == DeliveryDownloading {
			result = append(result, delivery)
			continue
		}
		token, err := randomToken(32)
		if err != nil {
			return result, err
		}
		tokenExpiry := artifact.ExpiresAt
		if maximum := now.Add(DefaultRetention); tokenExpiry.After(maximum) {
			tokenExpiry = maximum
		}
		queued, err := s.repository.SetDeliveryQueued(ctx, delivery.ID, digestToken(token), "", tokenExpiry, DeliveryQueued, now)
		if err != nil {
			return result, err
		}
		payload, err := json.Marshal(map[string]any{
			"artifact_id": artifact.ID, "delivery_id": delivery.ID, "filename": artifact.Filename, "content_type": artifact.ContentType,
			"cipher_size": artifact.CipherSize, "cipher_sha256": artifact.CipherSHA256, "plain_size": artifact.PlainSize,
			"plain_sha256": artifact.PlainSHA256, "ephemeral_public_key": artifact.EphemeralPublicKey,
			"wrapped_key": delivery.WrappedKey, "wrap_nonce": delivery.WrapNonce, "download_token": token,
			"download_path": "/v1/devices/" + delivery.TargetDeviceID + "/artifact-deliveries/" + delivery.ID + "/content",
			"result_path":   "/v1/devices/" + delivery.TargetDeviceID + "/artifact-deliveries/" + delivery.ID + "/result",
			"expires_at":    tokenExpiry.Format(time.RFC3339Nano), "conflict_policy": artifact.ConflictPolicy,
			"extract": artifact.ExtractRequested, "logical_target": artifact.LogicalTarget,
		})
		if err != nil {
			return result, err
		}
		command, _, err := s.commands.Enqueue(ctx, commands.EnqueueRequest{
			DeviceID: delivery.TargetDeviceID, Type: commands.TypeArtifactPull, Risk: devices.RiskMedium, Payload: payload,
			IdempotencyKey: "artifact:" + delivery.ID, Priority: 10, MaxAttempts: 3, NotBefore: now,
			ExpiresAt: tokenExpiry, CreatedBy: "artifact-relay",
		})
		if err != nil {
			failed, _ := s.repository.SetDeliveryResult(ctx, delivery.ID, DeliveryResultRequest{Status: DeliveryFailed, ErrorCode: "DISPATCH_FAILED", ErrorMessage: "artifact command could not be queued"}, now)
			result = append(result, failed)
			return result, err
		}
		queued, err = s.repository.SetDeliveryQueued(ctx, delivery.ID, digestToken(token), command.ID, tokenExpiry, DeliveryQueued, now)
		if err != nil {
			return result, err
		}
		result = append(result, queued)
	}
	return result, nil
}

func (s *Service) AuthorizeDownload(ctx context.Context, deviceID, deliveryID, token string) (DownloadGrant, error) {
	now := s.clock.Now().UTC()
	delivery, err := s.repository.GetDelivery(ctx, strings.TrimSpace(deliveryID))
	if err != nil {
		return DownloadGrant{}, err
	}
	if delivery.TargetDeviceID != strings.TrimSpace(deviceID) {
		return DownloadGrant{}, domainError(ErrDeliveryDeviceMismatch, "delivery does not belong to device")
	}
	if err := validateDeliveryToken(delivery, token, now); err != nil {
		return DownloadGrant{}, err
	}
	artifact, err := s.repository.GetArtifact(ctx, delivery.ArtifactID)
	if err != nil {
		return DownloadGrant{}, err
	}
	if artifact.Status != ArtifactUploaded || !now.Before(artifact.ExpiresAt) {
		return DownloadGrant{}, domainError(ErrInvalidState, "artifact is not available")
	}
	path, err := s.storagePath(artifact.StoragePath)
	if err != nil {
		return DownloadGrant{}, err
	}
	if _, err := os.Stat(path); err != nil {
		return DownloadGrant{}, domainError(ErrNotFound, "artifact payload is missing")
	}
	if delivery.Status == DeliveryQueued {
		if updated, markErr := s.repository.MarkDeliveryDownloading(ctx, delivery.ID, now); markErr == nil {
			delivery = updated
		}
	}
	return DownloadGrant{Artifact: artifact, Delivery: delivery, Path: path}, nil
}

func (s *Service) ReportDelivery(ctx context.Context, deviceID, deliveryID, token string, request DeliveryResultRequest) (Delivery, error) {
	now := s.clock.Now().UTC()
	delivery, err := s.repository.GetDelivery(ctx, strings.TrimSpace(deliveryID))
	if err != nil {
		return Delivery{}, err
	}
	if delivery.TargetDeviceID != strings.TrimSpace(deviceID) {
		return Delivery{}, domainError(ErrDeliveryDeviceMismatch, "delivery does not belong to device")
	}
	if err := validateDeliveryToken(delivery, token, now); err != nil {
		return Delivery{}, err
	}
	if request.Status != DeliveryCompleted && request.Status != DeliveryFailed {
		return Delivery{}, domainError(ErrValidation, "delivery result status must be completed or failed")
	}
	request.LocalPath = strings.TrimSpace(request.LocalPath)
	request.ErrorCode = strings.TrimSpace(request.ErrorCode)
	request.ErrorMessage = strings.TrimSpace(request.ErrorMessage)
	if len(request.LocalPath) > 4096 || len(request.ErrorCode) > 128 || len(request.ErrorMessage) > 4096 {
		return Delivery{}, domainError(ErrValidation, "delivery result field is too long")
	}
	updated, err := s.repository.SetDeliveryResult(ctx, delivery.ID, request, now)
	if err != nil {
		return Delivery{}, err
	}
	artifact, err := s.repository.GetArtifact(ctx, delivery.ArtifactID)
	if err == nil && artifact.DeleteAfterAllDelivered {
		items, listErr := s.repository.ListDeliveries(ctx, artifact.ID)
		if listErr == nil && allDeliveredOrAbandoned(items) {
			if path, pathErr := s.storagePath(artifact.StoragePath); pathErr == nil {
				_ = os.Remove(path)
				_ = os.Remove(filepath.Dir(path))
			}
			_ = s.repository.MarkArtifactDeleted(ctx, artifact.ID, now)
		}
	}
	return updated, nil
}

func (s *Service) Get(ctx context.Context, artifactID string) (UploadCompletion, error) {
	artifact, err := s.repository.GetArtifact(ctx, strings.TrimSpace(artifactID))
	if err != nil {
		return UploadCompletion{}, err
	}
	deliveries, err := s.repository.ListDeliveries(ctx, artifact.ID)
	if err != nil {
		return UploadCompletion{}, err
	}
	return UploadCompletion{Artifact: artifact, Deliveries: deliveries}, nil
}

func (s *Service) CleanupExpired(ctx context.Context) (int, error) {
	items, err := s.repository.ExpireBefore(ctx, s.clock.Now().UTC())
	if err != nil {
		return 0, err
	}
	for _, item := range items {
		if path, pathErr := s.storagePath(item.StoragePath); pathErr == nil {
			_ = os.Remove(path)
			_ = os.Remove(filepath.Dir(path))
		}
	}
	fetchCount, err := s.cleanupExpiredFetches(ctx)
	if err != nil {
		return len(items), err
	}
	return len(items) + fetchCount, nil
}

func (s *Service) RunCleanup(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.CleanupExpired(ctx)
		}
	}
}

func (s *Service) storagePath(relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", domainError(ErrValidation, "artifact storage path is invalid")
	}
	absolute := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, absolute)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", domainError(ErrValidation, "artifact storage path escapes root")
	}
	return absolute, nil
}

func validateFilename(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || strings.ContainsRune(value, 0) || filepath.Base(value) != value || strings.ContainsAny(value, `/\\`) || value == "." || value == ".." {
		return "", domainError(ErrValidation, "filename must be a safe basename")
	}
	return value, nil
}

func artifactPublicKey(snapshot devices.Snapshot) (string, error) {
	capabilities := snapshot.Device.Capabilities
	if snapshot.Heartbeat != nil {
		capabilities = snapshot.Heartbeat.Capabilities
	}
	for _, capability := range capabilities {
		if capability.Name != "artifact-relay" || !capability.Enabled {
			continue
		}
		key := strings.TrimSpace(capability.Metadata["x25519_public_key"])
		decoded, err := base64.RawURLEncoding.DecodeString(key)
		if err == nil && len(decoded) == 32 {
			return key, nil
		}
	}
	return "", domainError(ErrTargetKeyUnavailable, "target device %q has no usable artifact-relay X25519 key", snapshot.Device.ID)
}

func validateManifest(manifest UploadManifest, deliveries []Delivery) ([]Delivery, error) {
	if manifest.FormatVersion != "ADR1" || manifest.CipherAlgorithm != "AES-256-GCM-CHUNKED" {
		return nil, domainError(ErrValidation, "unsupported artifact encryption format")
	}
	if manifest.PlainSize < 0 || !validSHA256(manifest.PlainSHA256) {
		return nil, domainError(ErrValidation, "plain size or SHA-256 is invalid")
	}
	if value, err := base64.RawURLEncoding.DecodeString(manifest.EphemeralPublicKey); err != nil || len(value) != 32 {
		return nil, domainError(ErrValidation, "ephemeral X25519 public key is invalid")
	}
	byID := make(map[string]Delivery, len(deliveries))
	for _, delivery := range deliveries {
		byID[delivery.ID] = delivery
	}
	seen := map[string]struct{}{}
	result := make([]Delivery, 0, len(deliveries))
	for _, wrapped := range manifest.WrappedKeys {
		delivery, ok := byID[wrapped.DeliveryID]
		if !ok || delivery.TargetDeviceID != wrapped.TargetDeviceID {
			return nil, domainError(ErrValidation, "wrapped key target does not match delivery")
		}
		if _, duplicate := seen[delivery.ID]; duplicate {
			return nil, domainError(ErrValidation, "duplicate wrapped key for delivery")
		}
		seen[delivery.ID] = struct{}{}
		keyBytes, keyErr := base64.RawURLEncoding.DecodeString(wrapped.WrappedKey)
		nonceBytes, nonceErr := base64.RawURLEncoding.DecodeString(wrapped.WrapNonce)
		if keyErr != nil || len(keyBytes) != 48 || nonceErr != nil || len(nonceBytes) != 12 {
			return nil, domainError(ErrValidation, "wrapped key or nonce is invalid")
		}
		delivery.WrappedKey = wrapped.WrappedKey
		delivery.WrapNonce = wrapped.WrapNonce
		result = append(result, delivery)
	}
	if len(result) != len(deliveries) {
		return nil, domainError(ErrValidation, "manifest must contain one wrapped key per delivery")
	}
	return result, nil
}

func validateDeliveryToken(delivery Delivery, token string, now time.Time) error {
	if strings.TrimSpace(token) == "" || delivery.DownloadTokenDigest == "" {
		return domainError(ErrDeliveryTokenInvalid, "delivery token is required")
	}
	actual := digestToken(token)
	if len(actual) != len(delivery.DownloadTokenDigest) || subtle.ConstantTimeCompare([]byte(actual), []byte(delivery.DownloadTokenDigest)) != 1 {
		return domainError(ErrDeliveryTokenInvalid, "delivery token is invalid")
	}
	if delivery.DownloadTokenExpiresAt == nil || !now.Before(*delivery.DownloadTokenExpiresAt) {
		return domainError(ErrDeliveryTokenExpired, "delivery token expired")
	}
	return nil
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
func digestToken(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == sha256.Size
}
func allDeliveredOrAbandoned(items []Delivery) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.Status != DeliveryCompleted && item.Status != DeliveryCancelled && item.Status != DeliveryExpired {
			return false
		}
	}
	return true
}
