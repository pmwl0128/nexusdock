package httpx

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/uvwt/nexusdock/internal/agentdock"
)

func TestRuntimeBridgeErrorPreservesRemoteErrorDetails(t *testing.T) {
	remote := &agentdock.RemoteError{
		Code: "INVALID_LIMIT", Message: "limit must be an integer between 0 and 200",
		Category: "validation", Retryable: true,
		Details: map[string]any{"minimum": float64(0), "maximum": float64(200)},
	}

	converted := runtimeBridgeError(remote)
	if converted.Code != "AGENTDOCK_RUNTIME_REQUEST_FAILED" || converted.UpstreamCode != remote.Code {
		t.Fatalf("converted codes = %#v", converted)
	}
	if converted.Status != http.StatusBadRequest || converted.Category != remote.Category || converted.Retryable != remote.Retryable {
		t.Fatalf("converted semantics = %#v", converted)
	}
	if !reflect.DeepEqual(converted.Details, remote.Details) {
		t.Fatalf("details = %#v, want %#v", converted.Details, remote.Details)
	}

	payload := runtimeUnavailablePayload(converted)
	detail := payload["error"].(map[string]any)
	if detail["upstream_code"] != remote.Code || detail["category"] != remote.Category || detail["retryable"] != true {
		t.Fatalf("payload detail = %#v", detail)
	}
	if !reflect.DeepEqual(detail["details"], remote.Details) {
		t.Fatalf("payload details = %#v", detail["details"])
	}
	if runtimeErrorHTTPStatus(converted) != http.StatusBadRequest {
		t.Fatalf("status = %d", runtimeErrorHTTPStatus(converted))
	}
}

func TestRuntimeBridgeErrorMapsRemoteCategories(t *testing.T) {
	for _, test := range []struct {
		category string
		want     int
	}{
		{category: "validation", want: http.StatusBadRequest},
		{category: "not_found", want: http.StatusNotFound},
		{category: "conflict", want: http.StatusConflict},
		{category: "runtime", want: http.StatusInternalServerError},
	} {
		t.Run(test.category, func(t *testing.T) {
			converted := runtimeBridgeError(&agentdock.RemoteError{Code: "UPSTREAM", Message: "failed", Category: test.category})
			if got := runtimeErrorHTTPStatus(converted); got != test.want {
				t.Fatalf("status = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRuntimeBridgeErrorKeepsTransportFailuresUnavailable(t *testing.T) {
	converted := runtimeBridgeError(agentdock.ErrNodeOffline)
	if converted.Code != "AGENTDOCK_RUNTIME_UNREACHABLE" || converted.UpstreamCode != "" {
		t.Fatalf("converted = %#v", converted)
	}
	if got := runtimeErrorHTTPStatus(converted); got != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", got, http.StatusServiceUnavailable)
	}
}

func TestAgentDockRuntimeRequestTimeoutIsEightSeconds(t *testing.T) {
	if agentDockRuntimeRequestTimeout != 8*time.Second {
		t.Fatalf("runtime request timeout = %s", agentDockRuntimeRequestTimeout)
	}
}

func TestRuntimeUnavailableRecognizesRuntimeError(t *testing.T) {
	if !isRuntimeUnavailable(agentDockRuntimeError{Code: "AGENTDOCK_RUNTIME_UNREACHABLE"}) {
		t.Fatal("agentDockRuntimeError should be recognized")
	}
	if isRuntimeUnavailable(errors.New("other")) {
		t.Fatal("plain error should not be recognized")
	}
}
