package httpx

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/auth"
	"github.com/uvwt/agentdock-nexus/internal/core"
)

const sessionCookieName = "nexus_session"

type webSessionContextKey struct{}

func withWebSessionContext(ctx context.Context, session auth.WebSession) context.Context {
	return context.WithValue(ctx, webSessionContextKey{}, session)
}

func webSessionFromContext(ctx context.Context) (auth.WebSession, bool) {
	session, ok := ctx.Value(webSessionContextKey{}).(auth.WebSession)
	return session, ok
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "initialized": false})
		return
	}
	status, err := s.auth.AdminStatus(r.Context())
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "AUTH_STATUS_FAILED", "unable to read authentication status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "initialized": status.Initialized})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.loginTransportAllowed(r) {
		writeAuthError(w, http.StatusBadRequest, "HTTPS_REQUIRED", "login requires HTTPS")
		return
	}
	if !s.sameOrigin(r) {
		writeAuthError(w, http.StatusForbidden, "ORIGIN_REJECTED", "request origin is not allowed")
		return
	}
	status, err := s.auth.AdminStatus(r.Context())
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "AUTH_STATUS_FAILED", "unable to read authentication status")
		return
	}
	if !status.Initialized {
		writeAuthError(w, http.StatusServiceUnavailable, "ADMIN_NOT_INITIALIZED", "administrator is not initialized")
		return
	}
	var request struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		RememberMe bool   `json:"remember_me"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	issued, err := s.auth.Login(r.Context(), request.Username, request.Password, s.clientIPPrefix(r), summarizeUserAgent(r.UserAgent()), request.RememberMe)
	if err != nil {
		var limited *auth.RateLimitError
		if errors.As(err, &limited) {
			seconds := int(limited.RetryAfter.Round(time.Second).Seconds())
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeAuthError(w, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "too many login attempts; try again later")
			return
		}
		if core.ErrorCodeOf(err) == core.CodeInvalidToken {
			writeAuthError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password")
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "LOGIN_FAILED", "unable to create login session")
		return
	}
	s.setSessionCookie(w, r, issued.Token, issued.Session.RememberMe, issued.Session.AbsoluteExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": issued.Session})
}

func (s *Server) currentSession(w http.ResponseWriter, r *http.Request) {
	session, _ := webSessionFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "session": session})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	session, _ := webSessionFromContext(r.Context())
	err := s.auth.RevokeWebSession(r.Context(), session.UserID, session.ID, "logout")
	if err != nil && core.ErrorCodeOf(err) != core.CodeNotFound {
		writeAuthError(w, http.StatusInternalServerError, "LOGOUT_FAILED", "unable to revoke session")
		return
	}
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) updateCredential(w http.ResponseWriter, r *http.Request) {
	session, _ := webSessionFromContext(r.Context())
	var request struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	if err := s.auth.UpdateSecret(r.Context(), session.UserID, request.Current, request.Next); err != nil {
		switch core.ErrorCodeOf(err) {
		case core.CodeInvalidToken:
			writeAuthError(w, http.StatusUnauthorized, "CURRENT_CREDENTIAL_INVALID", "current credential is incorrect")
		case core.CodeValidation:
			writeAuthError(w, http.StatusBadRequest, "CREDENTIAL_POLICY_FAILED", publicCodedMessage(err, "new credential does not meet policy"))
		default:
			writeAuthError(w, http.StatusInternalServerError, "CREDENTIAL_UPDATE_FAILED", "unable to update credential")
		}
		return
	}
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reauthenticate": true})
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	session, _ := webSessionFromContext(r.Context())
	items, err := s.auth.ListWebSessions(r.Context(), session.UserID, session.ID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "SESSION_LIST_FAILED", "unable to list sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	current, _ := webSessionFromContext(r.Context())
	target := r.PathValue("sessionID")
	if target == current.ID {
		writeAuthError(w, http.StatusBadRequest, "USE_LOGOUT", "use logout for the current session")
		return
	}
	if err := s.auth.RevokeWebSession(r.Context(), current.UserID, target, "user_revoked"); err != nil {
		if core.ErrorCodeOf(err) == core.CodeNotFound {
			writeAuthError(w, http.StatusNotFound, "SESSION_NOT_FOUND", "active session not found")
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "SESSION_REVOKE_FAILED", "unable to revoke session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) logoutOtherSessions(w http.ResponseWriter, r *http.Request) {
	current, _ := webSessionFromContext(r.Context())
	count, err := s.auth.RevokeOtherWebSessions(r.Context(), current.UserID, current.ID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "SESSION_REVOKE_FAILED", "unable to revoke other sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": count})
}

func (s *Server) withUIAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			next(w, r)
			return
		}
		session, err := s.authenticateCookie(r)
		if err != nil {
			http.Redirect(w, r, "/login?return_to="+url.QueryEscape(safeReturnTo(r.URL.RequestURI())), http.StatusFound)
			return
		}
		if session.MustChangePassword && r.URL.Path != "/change-password" {
			http.Redirect(w, r, "/change-password?return_to="+url.QueryEscape(safeReturnTo(r.URL.RequestURI())), http.StatusFound)
			return
		}
		next(w, r.WithContext(withWebSessionContext(r.Context(), session)))
	}
}

func (s *Server) withWebSession(next http.HandlerFunc, allowCredentialUpdate bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := s.authenticateCookie(r)
		if err != nil {
			writeAuthError(w, http.StatusUnauthorized, "SESSION_REQUIRED", "login session is missing or expired")
			return
		}
		if session.MustChangePassword && !allowCredentialUpdate {
			writeAuthError(w, http.StatusForbidden, "CREDENTIAL_UPDATE_REQUIRED", "administrator credential must be updated before continuing")
			return
		}
		if isUnsafeMethod(r.Method) {
			if !s.sameOrigin(r) {
				writeAuthError(w, http.StatusForbidden, "ORIGIN_REJECTED", "request origin is not allowed")
				return
			}
			if !s.auth.VerifySessionCSRF(session, r.Header.Get("X-CSRF-Token")) {
				writeAuthError(w, http.StatusForbidden, "CSRF_REJECTED", "CSRF token is missing or invalid")
				return
			}
		}
		next(w, r.WithContext(withWebSessionContext(r.Context(), session)))
	}
}

func (s *Server) withAPIAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		cfg := s.cfg
		s.mu.RUnlock()
		if bearerMatches(r.Header.Get("Authorization"), cfg.AuthToken) {
			next(w, r)
			return
		}
		if s.auth == nil {
			if cfg.AuthToken == "" {
				next(w, r)
				return
			}
			writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid bearer token")
			return
		}
		s.withWebSession(next, false)(w, r)
	}
}

func bearerMatches(header, expected string) bool {
	if expected == "" || !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return false
	}
	actual := strings.TrimSpace(header[7:])
	return len(actual) == len(expected) && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func (s *Server) authenticateCookie(r *http.Request) (auth.WebSession, error) {
	if s.auth == nil {
		return auth.WebSession{}, core.NewError(core.CodeAuthRequired, "web authentication is not configured", nil)
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return auth.WebSession{}, core.NewError(core.CodeAuthRequired, "session is required", err)
	}
	return s.auth.AuthenticateWebSession(r.Context(), cookie.Value)
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, value string, remember bool, expires time.Time) {
	cookie := &http.Cookie{
		Name: sessionCookieName, Value: value, Path: "/", HttpOnly: true,
		Secure: s.secureRequest(r), SameSite: http.SameSiteStrictMode,
	}
	if remember {
		cookie.Expires = expires
		cookie.MaxAge = int(time.Until(expires).Seconds())
	}
	http.SetCookie(w, cookie)
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: s.secureRequest(r), SameSite: http.SameSiteStrictMode,
		Expires: time.Unix(1, 0), MaxAge: -1,
	})
}

func (s *Server) secureRequest(r *http.Request) bool {
	s.mu.RLock()
	allowInsecure := s.cfg.AuthAllowInsecureHTTP
	s.mu.RUnlock()
	if allowInsecure {
		return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
	if r.TLS != nil {
		return true
	}
	return s.isTrustedProxy(r) && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (s *Server) loginTransportAllowed(r *http.Request) bool {
	if s.secureRequest(r) {
		return true
	}
	s.mu.RLock()
	allowInsecure := s.cfg.AuthAllowInsecureHTTP
	s.mu.RUnlock()
	return allowInsecure
}

func (s *Server) sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	scheme := "http"
	host := r.Host
	if r.TLS != nil {
		scheme = "https"
	}
	if s.isTrustedProxy(r) {
		if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); value != "" {
			scheme = value
		}
		if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); value != "" {
			host = value
		}
	}
	return strings.EqualFold(parsed.Scheme, scheme) && strings.EqualFold(parsed.Host, host)
}

func (s *Server) isTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	s.mu.RLock()
	trusted := append([]string(nil), s.cfg.TrustedProxies...)
	s.mu.RUnlock()
	for _, entry := range trusted {
		if _, prefix, err := net.ParseCIDR(entry); err == nil {
			if prefix.Contains(ip) {
				return true
			}
			continue
		}
		if candidate := net.ParseIP(strings.Trim(entry, "[]")); candidate != nil && candidate.Equal(ip) {
			return true
		}
	}
	return false
}

func (s *Server) clientIPPrefix(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if s.isTrustedProxy(r) {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			host = forwarded
		} else if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			host = realIP
		}
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return "unknown"
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return net.IPv4(ipv4[0], ipv4[1], ipv4[2], 0).String() + "/24"
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func summarizeUserAgent(value string) string {
	browser := "Other browser"
	switch {
	case strings.Contains(value, "Edg/"):
		browser = "Edge"
	case strings.Contains(value, "Chrome/") || strings.Contains(value, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(value, "Firefox/") || strings.Contains(value, "FxiOS/"):
		browser = "Firefox"
	case strings.Contains(value, "Safari/"):
		browser = "Safari"
	}
	platform := "Unknown OS"
	switch {
	case strings.Contains(value, "iPhone") || strings.Contains(value, "iPad"):
		platform = "iOS"
	case strings.Contains(value, "Mac OS X") || strings.Contains(value, "Macintosh"):
		platform = "macOS"
	case strings.Contains(value, "Android"):
		platform = "Android"
	case strings.Contains(value, "Windows"):
		platform = "Windows"
	case strings.Contains(value, "Linux"):
		platform = "Linux"
	}
	return browser + " / " + platform
}

func safeReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\r\n") {
		return "/ui/"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil {
		return "/ui/"
	}
	return value
}

func isUnsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": map[string]any{"code": code, "message": message}})
}

func publicCodedMessage(err error, fallback string) string {
	var coded *core.CodedError
	if errors.As(err, &coded) && coded.Message != "" {
		return coded.Message
	}
	return fallback
}

func (s *Server) registerWebAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", s.uiIndex)
	mux.HandleFunc("GET /ui/assets/", s.uiApp)
	mux.HandleFunc("GET /change-password", s.withUIAccess(s.uiIndex))
	mux.HandleFunc("GET /v1/auth/status", s.authStatus)
	mux.HandleFunc("POST /v1/auth/login", s.login)
	mux.HandleFunc("GET /v1/auth/session", s.withWebSession(s.currentSession, true))
	mux.HandleFunc("POST /v1/auth/logout", s.withWebSession(s.logout, true))
	mux.HandleFunc("POST /v1/auth/credential", s.withWebSession(s.updateCredential, true))
	mux.HandleFunc("GET /v1/auth/sessions", s.withWebSession(s.listSessions, false))
	mux.HandleFunc("DELETE /v1/auth/sessions/{sessionID}", s.withWebSession(s.revokeSession, false))
	mux.HandleFunc("POST /v1/auth/sessions/logout-others", s.withWebSession(s.logoutOtherSessions, false))
}
