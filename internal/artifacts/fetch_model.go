package artifacts

import "time"

type FetchStatus string

const (
	FetchPending   FetchStatus = "pending"
	FetchQueued    FetchStatus = "queued"
	FetchListing   FetchStatus = "listing"
	FetchListed    FetchStatus = "listed"
	FetchUploading FetchStatus = "uploading"
	FetchReady     FetchStatus = "ready"
	FetchMounted   FetchStatus = "mounted"
	FetchFailed    FetchStatus = "failed"
	FetchExpired   FetchStatus = "expired"
)

func (s FetchStatus) Terminal() bool {
	return s == FetchMounted || s == FetchFailed || s == FetchExpired || s == FetchListed
}

type FetchJob struct {
	ID                     string       `json:"id"`
	RequesterDeviceID      string       `json:"requester_device_id"`
	SourceDeviceID         string       `json:"source_device_id"`
	SourcePath             string       `json:"source_path"`
	ArchiveRequested       bool         `json:"archive_requested"`
	Status                 FetchStatus  `json:"status"`
	Filename               string       `json:"filename,omitempty"`
	ContentType            string       `json:"content_type,omitempty"`
	StoragePath            string       `json:"-"`
	ReceiverPublicKey      string       `json:"-"`
	EphemeralPublicKey     string       `json:"ephemeral_public_key,omitempty"`
	WrappedKey             string       `json:"-"`
	WrapNonce              string       `json:"-"`
	PlainSize              int64        `json:"plain_size"`
	PlainSHA256            string       `json:"plain_sha256,omitempty"`
	CipherSize             int64        `json:"cipher_size"`
	CipherSHA256           string       `json:"cipher_sha256,omitempty"`
	UploadTokenDigest      string       `json:"-"`
	UploadTokenExpiresAt   time.Time    `json:"-"`
	UploadTokenUsedAt      *time.Time   `json:"-"`
	DownloadTokenDigest    string       `json:"-"`
	DownloadTokenExpiresAt time.Time    `json:"-"`
	CommandID              string       `json:"command_id,omitempty"`
	Listing                []FetchEntry `json:"listing,omitempty"`
	ErrorCode              string       `json:"error_code,omitempty"`
	ErrorMessage           string       `json:"error_message,omitempty"`
	ExpiresAt              time.Time    `json:"expires_at"`
	CreatedAt              time.Time    `json:"created_at"`
	UpdatedAt              time.Time    `json:"updated_at"`
	MountedAt              *time.Time   `json:"mounted_at,omitempty"`
}

type FetchEntry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Type       string    `json:"type"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type CreateFetchRequest struct {
	SourceDeviceID    string `json:"source_device_id"`
	SourcePath        string `json:"source_path"`
	Archive           bool   `json:"archive,omitempty"`
	ReceiverPublicKey string `json:"receiver_public_key"`
	RetentionSeconds  int64  `json:"retention_seconds,omitempty"`
}

type CreateFetchResult struct {
	Fetch         FetchJob `json:"fetch"`
	DownloadToken string   `json:"download_token"`
}

type FetchManifest struct {
	FormatVersion      string `json:"format_version"`
	CipherAlgorithm    string `json:"cipher_algorithm"`
	Filename           string `json:"filename"`
	ContentType        string `json:"content_type"`
	PlainSize          int64  `json:"plain_size"`
	PlainSHA256        string `json:"plain_sha256"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	WrappedKey         string `json:"wrapped_key"`
	WrapNonce          string `json:"wrap_nonce"`
}

type FetchResultRequest struct {
	Status       FetchStatus  `json:"status"`
	Listing      []FetchEntry `json:"listing,omitempty"`
	ErrorCode    string       `json:"error_code,omitempty"`
	ErrorMessage string       `json:"error_message,omitempty"`
}

type FetchDownloadGrant struct {
	Fetch FetchJob `json:"fetch"`
	Path  string   `json:"-"`
}
