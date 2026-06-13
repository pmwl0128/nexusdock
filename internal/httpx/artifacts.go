package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock-nexus/internal/artifacts"
)

const maxArtifactManifestBytes = 1 << 20

func (s *Server) registerArtifactRoutes(mux *http.ServeMux) {
	admin := func(next http.HandlerFunc) http.HandlerFunc { return s.withAPIAccess(next) }
	mux.HandleFunc("POST /v1/artifacts/uploads", admin(s.createArtifactUpload))
	mux.HandleFunc("GET /v1/artifacts/{artifactId}", admin(s.getArtifact))
	mux.HandleFunc("POST /v1/artifacts/{artifactId}/dispatch", admin(s.dispatchArtifact))
	mux.HandleFunc("POST /v1/artifacts/{artifactId}/content", s.uploadArtifactContent)
	mux.HandleFunc("POST /v1/devices/{deviceId}/artifacts/uploads", s.createDeviceArtifactUpload)
	mux.HandleFunc("GET /v1/devices/{deviceId}/artifact-deliveries/{deliveryId}/content", s.downloadArtifactContent)
	mux.HandleFunc("POST /v1/devices/{deviceId}/artifact-deliveries/{deliveryId}/result", s.completeArtifactDelivery)
}

func (s *Server) createArtifactUpload(w http.ResponseWriter, r *http.Request) {
	var request artifacts.CreateUploadRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.artifacts.CreateUpload(r.Context(), "api", "", request)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) createDeviceArtifactUpload(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	var request artifacts.CreateUploadRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.artifacts.CreateUpload(r.Context(), "device", deviceID, request)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) getArtifact(w http.ResponseWriter, r *http.Request) {
	result, err := s.artifacts.Get(r.Context(), r.PathValue("artifactId"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) dispatchArtifact(w http.ResponseWriter, r *http.Request) {
	result, err := s.artifacts.Dispatch(r.Context(), r.PathValue("artifactId"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": result})
}

func (s *Server) uploadArtifactContent(w http.ResponseWriter, r *http.Request) {
	uploadToken := strings.TrimSpace(r.Header.Get("X-Artifact-Upload-Token"))
	lease, err := s.artifacts.BeginUpload(r.Context(), r.PathValue("artifactId"), uploadToken)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	completed := false
	defer func() {
		if !completed {
			s.artifacts.AbortUpload(r.Context(), lease)
		}
	}()

	r.Body = http.MaxBytesReader(w, r.Body, s.artifacts.MaxCipherBytes()+maxArtifactManifestBytes+(2<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "multipart/form-data is required"})
		return
	}
	var manifest artifacts.UploadManifest
	var manifestSeen, fileSeen bool
	var cipherSize int64
	hash := sha256.New()
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "invalid multipart upload"})
			return
		}
		switch part.FormName() {
		case "manifest":
			if manifestSeen {
				part.Close()
				writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "manifest field must appear once"})
				return
			}
			manifestSeen = true
			data, readErr := io.ReadAll(io.LimitReader(part, maxArtifactManifestBytes+1))
			part.Close()
			if readErr != nil || len(data) > maxArtifactManifestBytes || json.Unmarshal(data, &manifest) != nil {
				writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "artifact manifest is invalid"})
				return
			}
		case "file":
			if fileSeen {
				part.Close()
				writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "file field must appear once"})
				return
			}
			fileSeen = true
			output, openErr := os.OpenFile(lease.TempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
			if openErr != nil {
				part.Close()
				writeArtifactError(w, openErr)
				return
			}
			written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(part, s.artifacts.MaxCipherBytes()+1))
			closeErr := output.Close()
			part.Close()
			cipherSize = written
			if copyErr != nil || closeErr != nil {
				writeArtifactError(w, fmt.Errorf("store artifact upload: %w", firstError(copyErr, closeErr)))
				return
			}
			if written > s.artifacts.MaxCipherBytes() {
				writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrTooLarge, Message: "encrypted artifact exceeds server limit"})
				return
			}
		default:
			part.Close()
			writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "unexpected multipart field"})
			return
		}
	}
	if !manifestSeen || !fileSeen || cipherSize == 0 {
		writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "manifest and non-empty file fields are required"})
		return
	}
	result, err := s.artifacts.CompleteUpload(r.Context(), lease, manifest, cipherSize, hex.EncodeToString(hash.Sum(nil)))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	completed = true
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) downloadArtifactContent(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	grant, err := s.artifacts.AuthorizeDownload(r.Context(), deviceID, r.PathValue("deliveryId"), r.Header.Get("X-Artifact-Delivery-Token"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	file, err := os.Open(grant.Path)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": grant.Artifact.Filename + ".adr"}))
	w.Header().Set("X-Artifact-Cipher-SHA256", grant.Artifact.CipherSHA256)
	w.Header().Set("X-Artifact-Plain-SHA256", grant.Artifact.PlainSHA256)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(grant.Path), info.ModTime(), file)
}

func (s *Server) completeArtifactDelivery(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	var request artifacts.DeliveryResultRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.artifacts.ReportDelivery(r.Context(), deviceID, r.PathValue("deliveryId"), r.Header.Get("X-Artifact-Delivery-Token"), request)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeArtifactError(w http.ResponseWriter, err error) {
	var artifactError *artifacts.Error
	if errors.As(err, &artifactError) {
		status := http.StatusInternalServerError
		switch artifactError.Code {
		case artifacts.ErrValidation:
			status = http.StatusBadRequest
		case artifacts.ErrNotFound, artifacts.ErrDeliveryNotFound, artifacts.ErrFetchNotFound:
			status = http.StatusNotFound
		case artifacts.ErrUploadTokenInvalid, artifacts.ErrDeliveryTokenInvalid, artifacts.ErrFetchTokenInvalid:
			status = http.StatusUnauthorized
		case artifacts.ErrDeliveryDeviceMismatch, artifacts.ErrTargetKeyUnavailable, artifacts.ErrFetchDeviceMismatch:
			status = http.StatusForbidden
		case artifacts.ErrUploadTokenExpired, artifacts.ErrDeliveryTokenExpired, artifacts.ErrFetchTokenExpired:
			status = http.StatusGone
		case artifacts.ErrTooLarge:
			status = http.StatusRequestEntityTooLarge
		case artifacts.ErrUploadAlreadyUsed, artifacts.ErrInvalidState, artifacts.ErrConflict:
			status = http.StatusConflict
		}
		writeNexusError(w, status, string(artifactError.Code), artifactError.Message)
		return
	}
	writeNexusError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "artifact operation failed")
}

func firstError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return errors.New("unknown artifact I/O error")
}
