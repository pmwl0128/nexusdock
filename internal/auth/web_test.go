package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/uvwt/nexusdock/internal/core"
)

type authTestClock struct {
	value time.Time
}

func (c *authTestClock) Now() time.Time {
	return c.value
}

func (c *authTestClock) Advance(duration time.Duration) {
	c.value = c.value.Add(duration)
}

func newWebAuthTestService(t *testing.T) (*Service, *sql.DB, *authTestClock) {
	t.Helper()

	ctx := context.Background()
	db, err := core.OpenSQLite(ctx, filepath.Join(t.TempDir(), "nexus.db"), 2)
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := core.NewMigrationRunner(db, nil).Run(ctx); err != nil {
		t.Fatalf("执行测试迁移失败: %v", err)
	}

	clock := &authTestClock{value: time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC)}
	service := NewService(db)
	service.now = clock.Now
	return service, db, clock
}

func TestPasswordArgon2AndValidation(t *testing.T) {
	hash, err := HashPasswordArgon2("correct horse battery staple")
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("密码哈希不能保存明文")
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("正确密码未通过校验")
	}
	if VerifyPassword("wrong password", hash) {
		t.Fatal("错误密码通过了校验")
	}
	if VerifyPassword("anything", "pbkdf2-sha256$210000$legacy$legacy") {
		t.Fatal("已退出的 PBKDF2 哈希不应继续通过校验")
	}

	tests := []struct {
		name     string
		username string
		password string
		wantCode core.ErrorCode
	}{
		{name: "长度不足", username: "owner", password: "short", wantCode: core.CodeValidation},
		{name: "与用户名相同", username: "OwnerAccount12", password: "owneraccount12", wantCode: core.CodeValidation},
		{name: "常见弱密码", username: "owner", password: "password1234", wantCode: core.CodeValidation},
		{name: "合格密码", username: "owner", password: "correct horse battery staple"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.username, tt.password)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidatePassword 返回了意外错误: %v", err)
				}
				return
			}
			if got := core.ErrorCodeOf(err); got != tt.wantCode {
				t.Fatalf("ValidatePassword 错误码 = %q, 期望 %q，错误: %v", got, tt.wantCode, err)
			}
		})
	}
}

func TestAdminLoginAndSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	service, _, clock := newWebAuthTestService(t)

	status, err := service.AdminStatus(ctx)
	if err != nil {
		t.Fatalf("读取初始管理员状态失败: %v", err)
	}
	if status.Initialized {
		t.Fatal("空数据库不应已有管理员")
	}
	if err := service.InitializeAdmin(ctx, "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("初始化管理员失败: %v", err)
	}
	if err := service.InitializeAdmin(ctx, "other", "another secure password"); core.ErrorCodeOf(err) != core.CodeDBConflict {
		t.Fatalf("重复初始化错误 = %v, 期望 DB_CONFLICT", err)
	}

	if _, err := service.Login(ctx, "owner", "wrong password", "192.0.2.0/24", "test-browser", false); core.ErrorCodeOf(err) != core.CodeInvalidToken {
		t.Fatalf("错误密码登录错误 = %v, 期望 INVALID_TOKEN", err)
	}
	// 首次失败会产生短暂节流，推进测试时钟后再验证成功登录链路。
	clock.Advance(time.Second)
	issued, err := service.Login(ctx, "OWNER", "correct horse battery staple", "192.0.2.0/24", "test-browser", false)
	if err != nil {
		t.Fatalf("正确密码登录失败: %v", err)
	}
	if issued.Token == "" || issued.Session.CSRFToken == "" {
		t.Fatalf("登录结果缺少会话凭据: %#v", issued.Session)
	}
	if issued.Session.IdleExpiresAt.Sub(issued.Session.CreatedAt) != 12*time.Hour {
		t.Fatalf("普通会话空闲窗口 = %v, 期望 12h", issued.Session.IdleExpiresAt.Sub(issued.Session.CreatedAt))
	}

	authenticated, err := service.AuthenticateWebSession(ctx, issued.Token)
	if err != nil {
		t.Fatalf("认证新会话失败: %v", err)
	}
	if authenticated.ID != issued.Session.ID || !service.VerifySessionCSRF(authenticated, issued.Session.CSRFToken) {
		t.Fatalf("认证后的会话信息不一致: %#v", authenticated)
	}

	sessions, err := service.ListWebSessions(ctx, authenticated.UserID, authenticated.ID)
	if err != nil {
		t.Fatalf("列出会话失败: %v", err)
	}
	if len(sessions) != 1 || !sessions[0].Current {
		t.Fatalf("活动会话列表 = %#v, 期望仅包含当前会话", sessions)
	}
	if err := service.RevokeWebSession(ctx, authenticated.UserID, authenticated.ID, "logout"); err != nil {
		t.Fatalf("撤销会话失败: %v", err)
	}
	if _, err := service.AuthenticateWebSession(ctx, issued.Token); core.ErrorCodeOf(err) != core.CodeTokenRevoked {
		t.Fatalf("撤销后认证错误 = %v, 期望 TOKEN_REVOKED", err)
	}
}

func TestListWebSessionsExcludesIdleExpiredSessions(t *testing.T) {
	ctx := context.Background()
	service, db, clock := newWebAuthTestService(t)
	if err := service.InitializeAdmin(ctx, "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("初始化管理员失败: %v", err)
	}

	first, err := service.Login(ctx, "owner", "correct horse battery staple", "192.0.2.0/24", "first-browser", false)
	if err != nil {
		t.Fatalf("创建第一个会话失败: %v", err)
	}
	second, err := service.Login(ctx, "owner", "correct horse battery staple", "198.51.100.0/24", "second-browser", true)
	if err != nil {
		t.Fatalf("创建第二个会话失败: %v", err)
	}

	// 模拟仅空闲过期、但尚未达到绝对过期时间的会话。
	if _, err := db.ExecContext(ctx, `UPDATE user_sessions SET idle_expires_at = ? WHERE id = ?`, clock.Now().Add(-time.Minute).Format(time.RFC3339Nano), first.Session.ID); err != nil {
		t.Fatalf("设置空闲过期时间失败: %v", err)
	}
	sessions, err := service.ListWebSessions(ctx, second.Session.UserID, second.Session.ID)
	if err != nil {
		t.Fatalf("列出会话失败: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != second.Session.ID || !sessions[0].Current {
		t.Fatalf("活动会话列表 = %#v, 期望排除空闲过期会话", sessions)
	}
}

func TestAuthenticateWebSessionRefreshesIdleWindow(t *testing.T) {
	ctx := context.Background()
	service, _, clock := newWebAuthTestService(t)
	if err := service.InitializeAdmin(ctx, "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("初始化管理员失败: %v", err)
	}
	issued, err := service.Login(ctx, "owner", "correct horse battery staple", "192.0.2.0/24", "test-browser", false)
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}

	clock.Advance(2 * time.Minute)
	refreshed, err := service.AuthenticateWebSession(ctx, issued.Token)
	if err != nil {
		t.Fatalf("刷新会话失败: %v", err)
	}
	if !refreshed.LastSeenAt.Equal(clock.Now()) {
		t.Fatalf("LastSeenAt = %v, 期望 %v", refreshed.LastSeenAt, clock.Now())
	}
	if !refreshed.IdleExpiresAt.Equal(clock.Now().Add(12 * time.Hour)) {
		t.Fatalf("IdleExpiresAt = %v, 期望 %v", refreshed.IdleExpiresAt, clock.Now().Add(12*time.Hour))
	}
}

func TestUpdatePasswordRevokesExistingSessions(t *testing.T) {
	ctx := context.Background()
	service, _, clock := newWebAuthTestService(t)
	if err := service.InitializeAdmin(ctx, "owner", "correct horse battery staple"); err != nil {
		t.Fatalf("初始化管理员失败: %v", err)
	}
	first, err := service.Login(ctx, "owner", "correct horse battery staple", "192.0.2.0/24", "first-browser", false)
	if err != nil {
		t.Fatalf("创建第一个会话失败: %v", err)
	}
	second, err := service.Login(ctx, "owner", "correct horse battery staple", "198.51.100.0/24", "second-browser", false)
	if err != nil {
		t.Fatalf("创建第二个会话失败: %v", err)
	}

	if err := service.UpdateSecret(ctx, first.Session.UserID, "correct horse battery staple", "a completely different secure password"); err != nil {
		t.Fatalf("修改密码失败: %v", err)
	}
	for _, token := range []string{first.Token, second.Token} {
		if _, err := service.AuthenticateWebSession(ctx, token); core.ErrorCodeOf(err) != core.CodeTokenRevoked {
			t.Fatalf("修改密码后旧会话错误 = %v, 期望 TOKEN_REVOKED", err)
		}
	}

	if _, err := service.Login(ctx, "owner", "correct horse battery staple", "203.0.113.0/24", "third-browser", false); core.ErrorCodeOf(err) != core.CodeInvalidToken {
		t.Fatalf("旧密码登录错误 = %v, 期望 INVALID_TOKEN", err)
	}
	clock.Advance(time.Second)
	if _, err := service.Login(ctx, "owner", "a completely different secure password", "203.0.113.0/24", "third-browser", false); err != nil {
		t.Fatalf("新密码登录失败: %v", err)
	}
}
