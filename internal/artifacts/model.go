package artifacts

import "time"

const (
	DefaultMaxCipherBytes = int64(500 << 20)
	DefaultRetention      = 24 * time.Hour
	MaximumRetention      = 7 * 24 * time.Hour
	MinimumRetention      = time.Hour
	DefaultUploadTTL      = 15 * time.Minute
)

type ArtifactStatus string

const (
	ArtifactPending   ArtifactStatus = "pending"
	ArtifactUploading ArtifactStatus = "uploading"
	ArtifactUploaded  ArtifactStatus = "uploaded"
	ArtifactExpired   ArtifactStatus = "expired"
	ArtifactDeleted   ArtifactStatus = "deleted"
)

type DeliveryStatus string

const (
	DeliveryPending     DeliveryStatus = "pending"
	DeliveryQueued      DeliveryStatus = "queued"
	DeliveryDownloading DeliveryStatus = "downloading"
	DeliveryCompleted   DeliveryStatus = "completed"
	DeliveryFailed      DeliveryStatus = "failed"
	DeliveryExpired     DeliveryStatus = "expired"
	DeliveryCancelled   DeliveryStatus = "cancelled"
)

func (s DeliveryStatus) Terminal() bool {
	return s == DeliveryCompleted || s == DeliveryFailed || s == DeliveryExpired || s == DeliveryCancelled
}

type Artifact struct {
	ID                      string         `json:"id"`
	SourceKind              string         `json:"source_kind"`
	SourceID                string         `json:"source_id,omitempty"`
	Filename                string         `json:"filename"`
	ContentType             string         `json:"content_type"`
	Status                  ArtifactStatus `json:"status"`
	StoragePath             string         `json:"-"`
	CipherSize              int64          `json:"cipher_size"`
	CipherSHA256            string         `json:"cipher_sha256,omitempty"`
	PlainSize               int64          `json:"plain_size"`
	PlainSHA256             string         `json:"plain_sha256,omitempty"`
	EphemeralPublicKey      string         `json:"ephemeral_public_key,omitempty"`
	UploadTokenDigest       string         `json:"-"`
	UploadTokenExpiresAt    time.Time      `json:"upload_token_expires_at"`
	UploadTokenUsedAt       *time.Time     `json:"upload_token_used_at,omitempty"`
	ExpiresAt               time.Time      `json:"expires_at"`
	DispatchRequested       bool           `json:"dispatch_requested"`
	DeleteAfterAllDelivered bool           `json:"delete_after_all_delivered"`
	ConflictPolicy          string         `json:"conflict_policy"`
	ExtractRequested        bool           `json:"extract_requested"`
	LogicalTarget           string         `json:"logical_target"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

type Delivery struct {
	ID                     string         `json:"id"`
	ArtifactID             string         `json:"artifact_id"`
	TargetDeviceID         string         `json:"target_device_id"`
	Status                 DeliveryStatus `json:"status"`
	WrappedKey             string         `json:"-"`
	WrapNonce              string         `json:"-"`
	DownloadTokenDigest    string         `json:"-"`
	DownloadTokenExpiresAt *time.Time     `json:"download_token_expires_at,omitempty"`
	CommandID              string         `json:"command_id,omitempty"`
	LocalPath              string         `json:"local_path,omitempty"`
	ErrorCode              string         `json:"error_code,omitempty"`
	ErrorMessage           string         `json:"error_message,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	CompletedAt            *time.Time     `json:"completed_at,omitempty"`
}

type CreateUploadRequest struct {
	Filename                string   `json:"filename"`
	ContentType             string   `json:"content_type,omitempty"`
	TargetDeviceIDs         []string `json:"target_device_ids"`
	Dispatch                *bool    `json:"dispatch,omitempty"`
	RetentionSeconds        int64    `json:"retention_seconds,omitempty"`
	DeleteAfterAllDelivered bool     `json:"delete_after_all_delivered,omitempty"`
	ConflictPolicy          string   `json:"conflict_policy,omitempty"`
	Extract                 bool     `json:"extract,omitempty"`
	LogicalTarget           string   `json:"logical_target,omitempty"`
}

type UploadTarget struct {
	DeliveryID      string `json:"delivery_id"`
	TargetDeviceID  string `json:"target_device_id"`
	X25519PublicKey string `json:"x25519_public_key"`
}

type CreateUploadResult struct {
	Artifact    Artifact       `json:"artifact"`
	Deliveries  []Delivery     `json:"deliveries"`
	Targets     []UploadTarget `json:"targets"`
	UploadToken string         `json:"upload_token"`
	UploadPath  string         `json:"upload_path"`
}

type WrappedKeyManifest struct {
	DeliveryID     string `json:"delivery_id"`
	TargetDeviceID string `json:"target_device_id"`
	WrappedKey     string `json:"wrapped_key"`
	WrapNonce      string `json:"wrap_nonce"`
}

type UploadManifest struct {
	FormatVersion      string               `json:"format_version"`
	CipherAlgorithm    string               `json:"cipher_algorithm"`
	PlainSize          int64                `json:"plain_size"`
	PlainSHA256        string               `json:"plain_sha256"`
	EphemeralPublicKey string               `json:"ephemeral_public_key"`
	WrappedKeys        []WrappedKeyManifest `json:"wrapped_keys"`
}

type UploadCompletion struct {
	Artifact   Artifact   `json:"artifact"`
	Deliveries []Delivery `json:"deliveries"`
}

type DeliveryResultRequest struct {
	Status       DeliveryStatus `json:"status"`
	LocalPath    string         `json:"local_path,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
}

type DownloadGrant struct {
	Artifact Artifact `json:"artifact"`
	Delivery Delivery `json:"delivery"`
	Path     string   `json:"-"`
}

type Detail struct {
	Artifact   Artifact   `json:"artifact"`
	Deliveries []Delivery `json:"deliveries"`
}
