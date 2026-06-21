package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/uvwt/agentdock-nexus/internal/audit"
	"github.com/uvwt/agentdock-nexus/internal/config"
	"github.com/uvwt/agentdock-nexus/internal/core"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltBytes   = 16
	argonKeyBytes    = 32
)

var commonWeakPasswords = map[string]struct{}{
	"123456789012": {}, "password1234": {}, "qwerty123456": {},
	"admin12345678": {}, "letmein123456": {}, "memorydock": {}, "recalldock": {},
}

type WebSession struct {
	ID                 string    `json:"id"`
	UserID             string    `json:"user_id"`
	Username           string    `json:"username"`
	DisplayName        string    `json:"display_name"`
	RememberMe         bool      `json:"remember_me"`
	IPPrefix           string    `json:"ip_prefix"`
	UserAgentSummary   string    `json:"user_agent_summary"`
	CreatedAt          time.Time `json:"created_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	IdleExpiresAt      time.Time `json:"idle_expires_at"`
	AbsoluteExpiresAt  time.Time `json:"absolute_expires_at"`
	MustChangePassword bool      `json:"must_change_password"`
	CSRFToken          string    `json:"csrf_token,omitempty"`
	Current            bool      `json:"current,omitempty"`
}

type IssuedWebSession struct {
	Session WebSession
	Token   string
}

type AdminStatus struct {
	Initialized bool   `json:"initialized"`
	Username    string `json:"username,omitempty"`
}

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return "too many login attempts" }

func HashPasswordArgon2(secret string) (string, error) {
	if secret == "" {
		return "", errors.New("password is required")
	}
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(secret), salt, argonIterations, argonMemory, argonParallelism, argonKeyBytes)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(secret, encoded string) (bool, bool) {
	if strings.HasPrefix(encoded, "$argon2id$") {
		ok := verifyArgon2(secret, encoded)
		return ok, false
	}
	if strings.HasPrefix(encoded, "pbkdf2-sha256$") {
		return config.VerifyPassword(secret, encoded), true
	}
	return false, false
}

func verifyArgon2(secret, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	if memory < 8*1024 || memory > 1024*1024 || iterations < 1 || iterations > 20 || parallelism < 1 || parallelism > 32 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func ValidatePassword(username, secret string) error {
	if len([]rune(secret)) < 12 {
		return core.NewError(core.CodeValidation, "password must be at least 12 characters", nil)
	}
	if len(secret) > 1024 {
		return core.NewError(core.CodeValidation, "password is too long", nil)
	}
	normalized := strings.ToLower(strings.TrimSpace(secret))
	if normalized == strings.ToLower(strings.TrimSpace(username)) {
		return core.NewError(core.CodeValidation, "password must not match the username", nil)
	}
	if _, weak := commonWeakPasswords[normalized]; weak {
		return core.NewError(core.CodeValidation, "password is too common", nil)
	}
	return nil
}

func (s *Service) AdminStatus(ctx context.Context) (AdminStatus, error) {
	var result AdminStatus
	err := s.db.QueryRowContext(ctx, `SELECT u.username FROM users u
		JOIN user_credentials c ON c.user_id = u.id
		WHERE u.status = 'active' ORDER BY u.created_at LIMIT 1`).Scan(&result.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read admin status: %w", err)
	}
	result.Initialized = true
	return result, nil
}

func (s *Service) EnsureLegacyAdmin(ctx context.Context, username, password, passwordHash string) (bool, error) {
	status, err := s.AdminStatus(ctx)
	if err != nil || status.Initialized {
		return false, err
	}
	username = strings.TrimSpace(username)
	passwordHash = strings.TrimSpace(passwordHash)
	if username == "" || (password == "" && passwordHash == "") {
		return false, nil
	}
	if username == "admin" && (password == "memorydock" || password == "recalldock") && passwordHash == "" {
		return false, nil
	}
	algorithm := "argon2id"
	encoded := passwordHash
	if encoded == "" {
		encoded, err = HashPasswordArgon2(password)
		if err != nil {
			return false, err
		}
	} else if strings.HasPrefix(encoded, "pbkdf2-sha256$") {
		algorithm = "pbkdf2-sha256"
	} else if strings.HasPrefix(encoded, "$argon2id$") {
		algorithm = "argon2id"
	} else {
		return false, errors.New("unsupported legacy password hash")
	}
	return true, s.createAdmin(ctx, username, encoded, algorithm, true)
}

func (s *Service) InitializeAdmin(ctx context.Context, username, password string) error {
	status, err := s.AdminStatus(ctx)
	if err != nil {
		return err
	}
	if status.Initialized {
		return core.NewError(core.CodeDBConflict, "administrator is already initialized", nil)
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return core.NewError(core.CodeValidation, "username is required", nil)
	}
	if err := ValidatePassword(username, password); err != nil {
		return err
	}
	encoded, err := HashPasswordArgon2(password)
	if err != nil {
		return err
	}
	return s.createAdmin(ctx, username, encoded, "argon2id", false)
}

func (s *Service) createAdmin(ctx context.Context, username, encoded, algorithm string, mustChange bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin admin initialization: %w", err)
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_credentials`).Scan(&count); err != nil {
		return fmt.Errorf("check admin credentials: %w", err)
	}
	if count > 0 {
		return core.NewError(core.CodeDBConflict, "administrator is already initialized", nil)
	}
	userID, err := core.NewID("usr")
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(id, username, display_name, status, created_at, updated_at)
		VALUES(?, ?, ?, 'active', ?, ?)`, userID, username, username, now, now); err != nil {
		return fmt.Errorf("create administrator: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_credentials(
		user_id, password_hash, password_algorithm, must_change_password, password_changed_at, created_at, updated_at
	) VALUES(?, ?, ?, ?, NULL, ?, ?)`, userID, encoded, algorithm, boolInt(mustChange), now, now); err != nil {
		return fmt.Errorf("create administrator credentials: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit administrator initialization: %w", err)
	}
	s.recordAuthEvent(ctx, core.Actor{Type: core.ActorSystem, ID: "auth"}, "auth.admin.initialize", userID, "succeeded", map[string]any{"migrated": mustChange})
	return nil
}

func (s *Service) Login(ctx context.Context, username, password, ipPrefix, userAgent string, remember bool) (IssuedWebSession, error) {
	username = strings.TrimSpace(username)
	accountKey := digestString(strings.ToLower(username))
	if retry, err := s.throttleRemaining(ctx, accountKey, ipPrefix); err != nil {
		return IssuedWebSession{}, err
	} else if retry > 0 {
		return IssuedWebSession{}, &RateLimitError{RetryAfter: retry}
	}

	var userID, storedUsername, displayName, status, encoded string
	var mustChange int
	err := s.db.QueryRowContext(ctx, `SELECT u.id, u.username, u.display_name, u.status,
		c.password_hash, c.must_change_password FROM users u
		JOIN user_credentials c ON c.user_id = u.id
		WHERE lower(u.username) = lower(?) LIMIT 1`, username).
		Scan(&userID, &storedUsername, &displayName, &status, &encoded, &mustChange)
	valid := false
	legacy := false
	if err == nil && status == "active" {
		valid, legacy = VerifyPassword(password, encoded)
	} else {
		_ = verifyArgon2(password, dummyArgonHash())
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IssuedWebSession{}, fmt.Errorf("read administrator credentials: %w", err)
	}
	if !valid {
		retry, recordErr := s.recordLoginFailure(ctx, accountKey, ipPrefix)
		s.recordAuthEvent(ctx, core.Actor{Type: core.ActorSystem, ID: "auth"}, "auth.login", digestString(strings.ToLower(username))[:16], "failed", map[string]any{"ip_prefix": ipPrefix})
		if recordErr != nil {
			return IssuedWebSession{}, recordErr
		}
		if retry >= time.Minute {
			return IssuedWebSession{}, &RateLimitError{RetryAfter: retry}
		}
		return IssuedWebSession{}, core.NewError(core.CodeInvalidToken, "invalid username or password", nil)
	}
	if legacy {
		upgraded, hashErr := HashPasswordArgon2(password)
		if hashErr != nil {
			return IssuedWebSession{}, hashErr
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE user_credentials SET password_hash = ?, password_algorithm = 'argon2id', updated_at = ? WHERE user_id = ?`, upgraded, s.now().UTC().Format(time.RFC3339Nano), userID); err != nil {
			return IssuedWebSession{}, fmt.Errorf("upgrade password hash: %w", err)
		}
	}
	if err := s.clearLoginFailures(ctx, accountKey, ipPrefix); err != nil {
		return IssuedWebSession{}, err
	}
	issued, err := s.createWebSession(ctx, userID, storedUsername, displayName, mustChange != 0, ipPrefix, userAgent, remember)
	if err != nil {
		return IssuedWebSession{}, err
	}
	s.recordAuthEvent(ctx, core.Actor{Type: core.ActorUser, ID: userID}, "auth.login", issued.Session.ID, "succeeded", map[string]any{"remember_me": remember, "ip_prefix": ipPrefix})
	return issued, nil
}

func (s *Service) createWebSession(ctx context.Context, userID, username, displayName string, mustChange bool, ipPrefix, userAgent string, remember bool) (IssuedWebSession, error) {
	raw, err := randomToken(32)
	if err != nil {
		return IssuedWebSession{}, err
	}
	csrfSalt, err := randomToken(24)
	if err != nil {
		return IssuedWebSession{}, err
	}
	id, err := core.NewID("ses")
	if err != nil {
		return IssuedWebSession{}, err
	}
	now := s.now().UTC()
	idleWindow, absoluteWindow := 12*time.Hour, 7*24*time.Hour
	if remember {
		idleWindow, absoluteWindow = 7*24*time.Hour, 30*24*time.Hour
	}
	session := WebSession{
		ID: id, UserID: userID, Username: username, DisplayName: displayName,
		RememberMe: remember, IPPrefix: ipPrefix, UserAgentSummary: userAgent,
		CreatedAt: now, LastSeenAt: now, IdleExpiresAt: now.Add(idleWindow),
		AbsoluteExpiresAt: now.Add(absoluteWindow), MustChangePassword: mustChange,
	}
	session.CSRFToken = deriveCSRF(raw, csrfSalt)
	_, err = s.db.ExecContext(ctx, `INSERT INTO user_sessions(
		id, user_id, token_hash, csrf_salt, remember_me, ip_prefix, user_agent_summary,
		created_at, last_seen_at, idle_expires_at, absolute_expires_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, userID, digestString(raw), csrfSalt, boolInt(remember), ipPrefix, userAgent,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), session.IdleExpiresAt.Format(time.RFC3339Nano), session.AbsoluteExpiresAt.Format(time.RFC3339Nano))
	if err != nil {
		return IssuedWebSession{}, fmt.Errorf("create web session: %w", err)
	}
	return IssuedWebSession{Session: session, Token: raw}, nil
}

func (s *Service) AuthenticateWebSession(ctx context.Context, raw string) (WebSession, error) {
	if strings.TrimSpace(raw) == "" {
		return WebSession{}, core.NewError(core.CodeAuthRequired, "session is required", nil)
	}
	var session WebSession
	var csrfSalt, created, lastSeen, idleExpiry, absoluteExpiry string
	var remember, mustChange int
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT s.id, s.user_id, u.username, u.display_name,
		s.remember_me, s.ip_prefix, s.user_agent_summary, s.created_at, s.last_seen_at,
		s.idle_expires_at, s.absolute_expires_at, s.revoked_at, s.csrf_salt,
		c.must_change_password
		FROM user_sessions s JOIN users u ON u.id = s.user_id
		JOIN user_credentials c ON c.user_id = s.user_id
		WHERE s.token_hash = ? AND u.status = 'active'`, digestString(raw)).Scan(
		&session.ID, &session.UserID, &session.Username, &session.DisplayName,
		&remember, &session.IPPrefix, &session.UserAgentSummary, &created, &lastSeen,
		&idleExpiry, &absoluteExpiry, &revoked, &csrfSalt, &mustChange)
	if errors.Is(err, sql.ErrNoRows) {
		return WebSession{}, core.NewError(core.CodeInvalidToken, "session is invalid", nil)
	}
	if err != nil {
		return WebSession{}, fmt.Errorf("lookup web session: %w", err)
	}
	if revoked.Valid {
		return WebSession{}, core.NewError(core.CodeTokenRevoked, "session has been revoked", nil)
	}
	session.RememberMe = remember != 0
	session.MustChangePassword = mustChange != 0
	if err := parseSessionTimes(&session, created, lastSeen, idleExpiry, absoluteExpiry); err != nil {
		return WebSession{}, err
	}
	now := s.now().UTC()
	if !now.Before(session.IdleExpiresAt) || !now.Before(session.AbsoluteExpiresAt) {
		_, _ = s.db.ExecContext(ctx, `UPDATE user_sessions SET revoked_at = ?, revoke_reason = 'expired' WHERE id = ? AND revoked_at IS NULL`, now.Format(time.RFC3339Nano), session.ID)
		return WebSession{}, core.NewError(core.CodeInvalidToken, "session has expired", nil)
	}
	idleWindow := 12 * time.Hour
	if session.RememberMe {
		idleWindow = 7 * 24 * time.Hour
	}
	newIdle := now.Add(idleWindow)
	if newIdle.After(session.AbsoluteExpiresAt) {
		newIdle = session.AbsoluteExpiresAt
	}
	if now.Sub(session.LastSeenAt) >= time.Minute {
		if _, err := s.db.ExecContext(ctx, `UPDATE user_sessions SET last_seen_at = ?, idle_expires_at = ? WHERE id = ? AND revoked_at IS NULL`, now.Format(time.RFC3339Nano), newIdle.Format(time.RFC3339Nano), session.ID); err != nil {
			return WebSession{}, fmt.Errorf("refresh web session: %w", err)
		}
		session.LastSeenAt = now
		session.IdleExpiresAt = newIdle
	}
	session.CSRFToken = deriveCSRF(raw, csrfSalt)
	return session, nil
}

func parseSessionTimes(session *WebSession, values ...string) error {
	targets := []*time.Time{&session.CreatedAt, &session.LastSeenAt, &session.IdleExpiresAt, &session.AbsoluteExpiresAt}
	for i, value := range values {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return fmt.Errorf("parse session timestamp: %w", err)
		}
		*targets[i] = parsed
	}
	return nil
}

func (s *Service) ListWebSessions(ctx context.Context, userID, currentID string) ([]WebSession, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, remember_me, ip_prefix, user_agent_summary,
		created_at, last_seen_at, idle_expires_at, absolute_expires_at
		FROM user_sessions WHERE user_id = ? AND revoked_at IS NULL AND absolute_expires_at > ?
		ORDER BY last_seen_at DESC`, userID, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("list web sessions: %w", err)
	}
	defer rows.Close()
	var result []WebSession
	for rows.Next() {
		var item WebSession
		var remember int
		var created, lastSeen, idleExpiry, absoluteExpiry string
		if err := rows.Scan(&item.ID, &remember, &item.IPPrefix, &item.UserAgentSummary, &created, &lastSeen, &idleExpiry, &absoluteExpiry); err != nil {
			return nil, fmt.Errorf("scan web session: %w", err)
		}
		item.UserID = userID
		item.RememberMe = remember != 0
		item.Current = item.ID == currentID
		if err := parseSessionTimes(&item, created, lastSeen, idleExpiry, absoluteExpiry); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Service) RevokeWebSession(ctx context.Context, userID, sessionID, reason string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE user_sessions SET revoked_at = ?, revoke_reason = ?
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, s.now().UTC().Format(time.RFC3339Nano), reason, sessionID, userID)
	if err != nil {
		return fmt.Errorf("revoke web session: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return core.NewError(core.CodeNotFound, "active session not found", nil)
	}
	s.recordAuthEvent(ctx, core.Actor{Type: core.ActorUser, ID: userID}, "auth.session.revoke", sessionID, "succeeded", nil)
	return nil
}

func (s *Service) RevokeOtherWebSessions(ctx context.Context, userID, currentID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE user_sessions SET revoked_at = ?, revoke_reason = 'logout_others'
		WHERE user_id = ? AND id <> ? AND revoked_at IS NULL`, s.now().UTC().Format(time.RFC3339Nano), userID, currentID)
	if err != nil {
		return 0, fmt.Errorf("revoke other web sessions: %w", err)
	}
	count, _ := result.RowsAffected()
	s.recordAuthEvent(ctx, core.Actor{Type: core.ActorUser, ID: userID}, "auth.session.revoke_others", userID, "succeeded", map[string]any{"count": count})
	return count, nil
}

func (s *Service) UpdateSecret(ctx context.Context, userID, currentPassword, newPassword string) error {
	var username, encoded string
	if err := s.db.QueryRowContext(ctx, `SELECT u.username, c.password_hash FROM users u JOIN user_credentials c ON c.user_id = u.id WHERE u.id = ?`, userID).Scan(&username, &encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.NewError(core.CodeNotFound, "administrator not found", nil)
		}
		return fmt.Errorf("read administrator password: %w", err)
	}
	valid, _ := VerifyPassword(currentPassword, encoded)
	if !valid {
		return core.NewError(core.CodeInvalidToken, "current password is incorrect", nil)
	}
	if err := ValidatePassword(username, newPassword); err != nil {
		return err
	}
	if same, _ := VerifyPassword(newPassword, encoded); same {
		return core.NewError(core.CodeValidation, "new password must be different", nil)
	}
	return s.replacePassword(ctx, userID, newPassword, "password_changed")
}

func (s *Service) RotateAdminCredential(ctx context.Context, username, newPassword string) error {
	var userID, storedUsername string
	query := `SELECT u.id, u.username FROM users u JOIN user_credentials c ON c.user_id = u.id WHERE u.status = 'active'`
	args := []any{}
	if strings.TrimSpace(username) != "" {
		query += ` AND lower(u.username) = lower(?)`
		args = append(args, strings.TrimSpace(username))
	}
	query += ` ORDER BY u.created_at LIMIT 1`
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&userID, &storedUsername); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.NewError(core.CodeNotFound, "administrator not found", nil)
		}
		return fmt.Errorf("read administrator: %w", err)
	}
	if err := ValidatePassword(storedUsername, newPassword); err != nil {
		return err
	}
	return s.replacePassword(ctx, userID, newPassword, "admin_reset")
}

func (s *Service) replacePassword(ctx context.Context, userID, newPassword, reason string) error {
	encoded, err := HashPasswordArgon2(newPassword)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin password update: %w", err)
	}
	defer tx.Rollback()
	now := s.now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE user_credentials SET password_hash = ?, password_algorithm = 'argon2id',
		must_change_password = 0, password_changed_at = ?, updated_at = ? WHERE user_id = ?`, encoded, now, now, userID)
	if err != nil {
		return fmt.Errorf("update administrator password: %w", err)
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		return core.NewError(core.CodeNotFound, "administrator not found", nil)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_sessions SET revoked_at = ?, revoke_reason = ? WHERE user_id = ? AND revoked_at IS NULL`, now, reason, userID); err != nil {
		return fmt.Errorf("revoke sessions after password update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit password update: %w", err)
	}
	s.recordAuthEvent(ctx, core.Actor{Type: core.ActorUser, ID: userID}, "auth.password.change", userID, "succeeded", map[string]any{"reason": reason})
	return nil
}

func (s *Service) VerifySessionCSRF(session WebSession, supplied string) bool {
	return supplied != "" && subtle.ConstantTimeCompare([]byte(session.CSRFToken), []byte(supplied)) == 1
}

func (s *Service) throttleRemaining(ctx context.Context, accountKey, ipPrefix string) (time.Duration, error) {
	var longest time.Duration
	for _, key := range [][2]string{{"account", accountKey}, {"ip", digestString(ipPrefix)}} {
		var blocked sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT blocked_until FROM login_throttles WHERE key_type = ? AND key_value = ?`, key[0], key[1]).Scan(&blocked)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("read login throttle: %w", err)
		}
		if blocked.Valid {
			until, err := time.Parse(time.RFC3339Nano, blocked.String)
			if err != nil {
				return 0, fmt.Errorf("parse login throttle: %w", err)
			}
			if remaining := until.Sub(s.now().UTC()); remaining > longest {
				longest = remaining
			}
		}
	}
	return longest, nil
}

func (s *Service) recordLoginFailure(ctx context.Context, accountKey, ipPrefix string) (time.Duration, error) {
	var longest time.Duration
	for _, key := range [][2]string{{"account", accountKey}, {"ip", digestString(ipPrefix)}} {
		var failures int
		err := s.db.QueryRowContext(ctx, `SELECT failures FROM login_throttles WHERE key_type = ? AND key_value = ?`, key[0], key[1]).Scan(&failures)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("read login failures: %w", err)
		}
		failures++
		delay := loginDelay(failures)
		if delay > longest {
			longest = delay
		}
		now := s.now().UTC()
		_, err = s.db.ExecContext(ctx, `INSERT INTO login_throttles(key_type, key_value, failures, blocked_until, last_failed_at)
			VALUES(?, ?, ?, ?, ?)
			ON CONFLICT(key_type, key_value) DO UPDATE SET failures = excluded.failures,
			blocked_until = excluded.blocked_until, last_failed_at = excluded.last_failed_at`,
			key[0], key[1], failures, now.Add(delay).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		if err != nil {
			return 0, fmt.Errorf("record login failure: %w", err)
		}
	}
	return longest, nil
}

func loginDelay(failures int) time.Duration {
	switch {
	case failures >= 10:
		return 15 * time.Minute
	case failures >= 7:
		return 5 * time.Minute
	case failures >= 6:
		return 2 * time.Minute
	case failures >= 5:
		return time.Minute
	default:
		return time.Duration(1<<max(0, failures-1)) * 250 * time.Millisecond
	}
}

func (s *Service) clearLoginFailures(ctx context.Context, accountKey, ipPrefix string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_throttles WHERE (key_type = 'account' AND key_value = ?) OR (key_type = 'ip' AND key_value = ?)`, accountKey, digestString(ipPrefix))
	if err != nil {
		return fmt.Errorf("clear login failures: %w", err)
	}
	return nil
}

func (s *Service) recordAuthEvent(ctx context.Context, actor core.Actor, action, objectID, result string, metadata map[string]any) {
	_, _ = audit.NewService(s.db).Record(ctx, audit.Event{
		Actor: actor, Action: action, ObjectType: "web_auth", ObjectID: objectID,
		Result: result, Risk: "high", Approval: "not_required", Metadata: metadata,
	})
}

func randomToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func digestString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

func deriveCSRF(sessionToken, salt string) string {
	mac := hmac.New(sha256.New, []byte(sessionToken))
	mac.Write([]byte(salt))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func dummyArgonHash() string {
	salt := []byte("nexus-dummy-salt")
	key := argon2.IDKey([]byte("not-the-password"), salt, argonIterations, argonMemory, argonParallelism, argonKeyBytes)
	return "$argon2id$v=19$m=" + strconv.Itoa(argonMemory) + ",t=" + strconv.Itoa(argonIterations) + ",p=" + strconv.Itoa(argonParallelism) + "$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(key)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
