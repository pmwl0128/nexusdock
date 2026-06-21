package httpx

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uvwt/agentdock-nexus/internal/config"
)

func TestSafeReturnToRejectsExternalAndControlValues(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "/ui/"},
		{name: "relative path", value: "/ui/#recall", want: "/ui/#recall"},
		{name: "absolute URL", value: "https://evil.example/ui/", want: "/ui/"},
		{name: "protocol relative", value: "//evil.example/ui/", want: "/ui/"},
		{name: "newline injection", value: "/ui/\r\nSet-Cookie: bad=1", want: "/ui/"},
		{name: "missing slash", value: "ui/#recall", want: "/ui/"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeReturnTo(tt.value); got != tt.want {
				t.Fatalf("safeReturnTo(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSameOriginHonorsTrustedProxyHeadersOnly(t *testing.T) {
	server := &Server{cfg: config.Config{TrustedProxies: []string{"10.0.0.0/8"}}}

	directTLS := httptest.NewRequest(http.MethodPost, "https://nexus.example/v1/auth/login", nil)
	directTLS.Header.Set("Origin", "https://nexus.example")
	if !server.sameOrigin(directTLS) {
		t.Fatalf("direct TLS same-origin request was rejected")
	}

	trustedProxy := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/auth/login", nil)
	trustedProxy.Host = "127.0.0.1"
	trustedProxy.RemoteAddr = "10.1.2.3:4567"
	trustedProxy.Header.Set("Origin", "https://nexus.example")
	trustedProxy.Header.Set("X-Forwarded-Proto", "https")
	trustedProxy.Header.Set("X-Forwarded-Host", "nexus.example")
	if !server.sameOrigin(trustedProxy) {
		t.Fatalf("trusted reverse proxy same-origin request was rejected")
	}

	untrustedProxy := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/auth/login", nil)
	untrustedProxy.RemoteAddr = "203.0.113.8:4567"
	untrustedProxy.Header.Set("Origin", "https://nexus.example")
	untrustedProxy.Header.Set("X-Forwarded-Proto", "https")
	untrustedProxy.Header.Set("X-Forwarded-Host", "nexus.example")
	if server.sameOrigin(untrustedProxy) {
		t.Fatalf("untrusted reverse proxy headers were accepted as same-origin")
	}
}

func TestSecureRequestRequiresTLSOrTrustedForwardedProto(t *testing.T) {
	server := &Server{cfg: config.Config{TrustedProxies: []string{"10.0.0.0/8"}}}

	directTLS := httptest.NewRequest(http.MethodGet, "https://nexus.example/", nil)
	directTLS.TLS = &tls.ConnectionState{}
	if !server.secureRequest(directTLS) {
		t.Fatalf("TLS request was not treated as secure")
	}

	trustedProxy := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	trustedProxy.RemoteAddr = "10.1.2.3:4567"
	trustedProxy.Header.Set("X-Forwarded-Proto", "https")
	if !server.secureRequest(trustedProxy) {
		t.Fatalf("trusted proxy https request was not treated as secure")
	}

	untrustedProxy := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	untrustedProxy.RemoteAddr = "203.0.113.8:4567"
	untrustedProxy.Header.Set("X-Forwarded-Proto", "https")
	if server.secureRequest(untrustedProxy) {
		t.Fatalf("untrusted forwarded proto marked request secure")
	}
}
