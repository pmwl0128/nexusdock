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

	"github.com/uvwt/agentdock-nexus/internal/artifacts"
)

func (s *Server) registerArtifactFetchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/devices/{deviceId}/artifact-fetches", s.createArtifactFetch)
	mux.HandleFunc("GET /v1/devices/{deviceId}/artifact-fetches/{fetchId}", s.getArtifactFetch)
	mux.HandleFunc("GET /v1/devices/{deviceId}/artifact-fetches/{fetchId}/content", s.downloadArtifactFetch)
	mux.HandleFunc("POST /v1/devices/{deviceId}/artifact-fetches/{fetchId}/mounted", s.confirmArtifactFetchMounted)
	mux.HandleFunc("POST /v1/devices/{deviceId}/artifact-fetches/{fetchId}/content", s.uploadArtifactFetch)
	mux.HandleFunc("POST /v1/devices/{deviceId}/artifact-fetches/{fetchId}/result", s.reportArtifactFetchResult)
}

func (s *Server) createArtifactFetch(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	var request artifacts.CreateFetchRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.artifacts.CreateFetch(r.Context(), deviceID, request)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) getArtifactFetch(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	result, err := s.artifacts.GetFetch(r.Context(), deviceID, r.PathValue("fetchId"), r.Header.Get("X-Artifact-Fetch-Token"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) reportArtifactFetchResult(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	var request artifacts.FetchResultRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	result, err := s.artifacts.ReportFetchResult(r.Context(), deviceID, r.PathValue("fetchId"), r.Header.Get("X-Artifact-Fetch-Upload-Token"), request)
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) uploadArtifactFetch(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	lease, err := s.artifacts.BeginFetchUpload(r.Context(), deviceID, r.PathValue("fetchId"), r.Header.Get("X-Artifact-Fetch-Upload-Token"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	completed := false
	defer func() {
		if !completed {
			s.artifacts.AbortFetchUpload(r.Context(), lease)
		}
	}()
	r.Body = http.MaxBytesReader(w, r.Body, s.artifacts.MaxCipherBytes()+maxArtifactManifestBytes+(2<<20))
	reader, err := r.MultipartReader()
	if err != nil {
		writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "multipart/form-data is required"})
		return
	}
	var manifest artifacts.FetchManifest
	var manifestSeen, fileSeen bool
	var size int64
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
				writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "fetch manifest is invalid"})
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
			size = written
			if copyErr != nil || closeErr != nil {
				writeArtifactError(w, fmt.Errorf("store fetch upload: %w", firstError(copyErr, closeErr)))
				return
			}
			if written > s.artifacts.MaxCipherBytes() {
				writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrTooLarge, Message: "encrypted fetch exceeds server limit"})
				return
			}
		default:
			part.Close()
			writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "unexpected multipart field"})
			return
		}
	}
	if !manifestSeen || !fileSeen || size == 0 {
		writeArtifactError(w, &artifacts.Error{Code: artifacts.ErrValidation, Message: "manifest and non-empty file fields are required"})
		return
	}
	result, err := s.artifacts.CompleteFetchUpload(r.Context(), lease, manifest, size, hex.EncodeToString(hash.Sum(nil)))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	completed = true
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) downloadArtifactFetch(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	grant, err := s.artifacts.AuthorizeFetchDownload(r.Context(), deviceID, r.PathValue("fetchId"), r.Header.Get("X-Artifact-Fetch-Token"))
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
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": grant.Fetch.Filename + ".adr"}))
	w.Header().Set("X-Artifact-Cipher-SHA256", grant.Fetch.CipherSHA256)
	w.Header().Set("X-Artifact-Plain-SHA256", grant.Fetch.PlainSHA256)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(grant.Path), info.ModTime(), file)
}

func (s *Server) confirmArtifactFetchMounted(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Authenticate(r.Context(), deviceID, bearerToken(r)); err != nil {
		writeControlPlaneError(w, err)
		return
	}
	result, err := s.artifacts.ConfirmFetchMounted(r.Context(), deviceID, r.PathValue("fetchId"), r.Header.Get("X-Artifact-Fetch-Token"))
	if err != nil {
		writeArtifactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
