package devices_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/devices"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

func enrollmentRequest(token, name, platform string) devices.EnrollmentRequest {
	return devices.EnrollmentRequest{
		Token: token, Name: name, Platform: platform, Arch: "arm64",
		AgentDockVersion: "1.0.0", PublicKey: "test-public-key",
	}
}

func validHeartbeat(now time.Time) devices.Heartbeat {
	return devices.Heartbeat{
		SentAt: now, UptimeSeconds: 60, AgentDockVersion: "1.2.3",
		Metrics:      devices.Metrics{CPUPercent: 20, MemoryPercent: 50, DiskPercent: 30},
		Capabilities: []devices.Capability{{Name: "browser", Version: "1", Enabled: true}},
		Skills:       []devices.SkillSummary{{Name: "health", Version: "1.0.0", Active: true}},
	}
}

func newApprovedDevice(t *testing.T, clock *fakeClock, policy devices.Policy) (*devices.Service, devices.EnrollmentResult) {
	t.Helper()
	service, err := devices.NewService(devices.NewMemoryRepository(), devices.WithClock(clock))
	if err != nil {
		t.Fatal(err)
	}
	token, err := service.CreateEnrollmentToken(context.Background(), "test-user", time.Hour, policy)
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := service.Enroll(context.Background(), enrollmentRequest(token.PlainToken, "DockAir", "linux"))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := service.Approve(context.Background(), enrolled.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	enrolled.Device = approved
	return service, enrolled
}

func TestEnrollmentTokenCanOnlyBeUsedOnce(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 6, 5, 15, 0, 0, 0, time.UTC)}
	service, err := devices.NewService(devices.NewMemoryRepository(), devices.WithClock(clock))
	if err != nil {
		t.Fatal(err)
	}
	token, err := service.CreateEnrollmentToken(context.Background(), "admin", time.Minute, devices.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Enroll(context.Background(), enrollmentRequest(token.PlainToken, "DockMini", "darwin"))
	if err != nil {
		t.Fatalf("first enrollment failed: %v", err)
	}
	if first.DeviceToken == "" || first.Device.DeviceTokenDigest == "" {
		t.Fatal("device credentials were not issued")
	}
	_, err = service.Enroll(context.Background(), enrollmentRequest(token.PlainToken, "Replay", "darwin"))
	if !devices.IsCode(err, devices.ErrEnrollmentTokenUsed) {
		t.Fatalf("expected one-time token rejection, got %v", err)
	}
	if first.Device.DeviceTokenDigest == first.DeviceToken {
		t.Fatal("plaintext device token was persisted")
	}
}

func TestHeartbeatStatusAndRevocation(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 6, 5, 16, 0, 0, 0, time.UTC)}
	service, enrolled := newApprovedDevice(t, clock, devices.DefaultPolicy())
	ctx := context.Background()

	updated, err := service.Heartbeat(ctx, enrolled.Device.ID, enrolled.DeviceToken, validHeartbeat(clock.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != devices.StatusOnline || updated.LastSeen == nil {
		t.Fatalf("unexpected heartbeat state: %+v", updated)
	}

	clock.Advance(90 * time.Second)
	changed, err := service.RefreshStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].Status != devices.StatusDegraded {
		t.Fatalf("expected degraded at 90 seconds, got %+v", changed)
	}

	clock.Advance(90 * time.Second)
	changed, err = service.RefreshStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].Status != devices.StatusOffline {
		t.Fatalf("expected offline at 180 seconds, got %+v", changed)
	}

	_, err = service.Revoke(ctx, enrolled.Device.ID, "compromised")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Heartbeat(ctx, enrolled.Device.ID, enrolled.DeviceToken, validHeartbeat(clock.Now()))
	if !devices.IsCode(err, devices.ErrDeviceRevoked) {
		t.Fatalf("revoked device should be rejected, got %v", err)
	}
}

func TestCommandQueueIdempotencyLeaseAndPolicy(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 6, 5, 17, 0, 0, 0, time.UTC)}
	policy := devices.Policy{
		AllowedCommandTypes: []string{string(commands.TypeHealthCheck)},
		MaxRisk:             devices.RiskLow,
		ReleaseChannel:      devices.ChannelStable,
		AllowedSkills:       []string{"health"},
	}
	deviceService, enrolled := newApprovedDevice(t, clock, policy)
	commandService, err := commands.NewService(
		commands.NewMemoryRepository(), deviceService,
		commands.WithClock(clock), commands.WithLeaseDuration(30*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	request := commands.EnqueueRequest{
		DeviceID: enrolled.Device.ID, Type: commands.TypeHealthCheck, Risk: devices.RiskLow,
		Payload: json.RawMessage(`{"deep":true}`), IdempotencyKey: "health-1",
		MaxAttempts: 2, ExpiresAt: clock.Now().Add(10 * time.Minute), CreatedBy: "agent-1",
	}
	first, created, err := commandService.Enqueue(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("enqueue failed: created=%v err=%v", created, err)
	}
	duplicate, created, err := commandService.Enqueue(context.Background(), request)
	if err != nil || created || duplicate.ID != first.ID {
		t.Fatalf("idempotency failed: first=%s duplicate=%s created=%v err=%v", first.ID, duplicate.ID, created, err)
	}

	lease, err := commandService.LeaseNext(context.Background(), enrolled.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Command.Status != commands.StatusLeased || lease.Command.Attempts != 1 {
		t.Fatalf("unexpected lease: %+v", lease)
	}
	_, err = commandService.Start(context.Background(), first.ID, "wrong-lease")
	if !commands.IsCode(err, commands.ErrLeaseMismatch) {
		t.Fatalf("wrong lease was not rejected: %v", err)
	}
	running, err := commandService.Start(context.Background(), first.ID, lease.ID)
	if err != nil || running.Status != commands.StatusRunning {
		t.Fatalf("start failed: %+v %v", running, err)
	}
	completed, err := commandService.Complete(context.Background(), first.ID, lease.ID, commands.Result{Success: true, Output: json.RawMessage(`{"ok":true}`)})
	if err != nil || completed.Status != commands.StatusSucceeded {
		t.Fatalf("complete failed: %+v %v", completed, err)
	}

	denied := request
	denied.IdempotencyKey = "restart-1"
	denied.Type = commands.TypeServiceRestart
	denied.Risk = devices.RiskHigh
	_, _, err = commandService.Enqueue(context.Background(), denied)
	if !devices.IsCode(err, devices.ErrPolicyDenied) {
		t.Fatalf("unauthorized command was not rejected: %v", err)
	}

	denied.Type = commands.Type("shell.exec")
	denied.IdempotencyKey = "shell-01"
	_, _, err = commandService.Enqueue(context.Background(), denied)
	if !commands.IsCode(err, commands.ErrCommandTypeDenied) {
		t.Fatalf("arbitrary shell command was not rejected: %v", err)
	}
}

func TestExpiredLeaseCanBeReclaimedWithoutDuplicateCompletion(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 6, 5, 18, 0, 0, 0, time.UTC)}
	policy := devices.Policy{
		AllowedCommandTypes: []string{string(commands.TypeDiagnosticsCollect)},
		MaxRisk:             devices.RiskLow, ReleaseChannel: devices.ChannelCanary,
	}
	deviceService, enrolled := newApprovedDevice(t, clock, policy)
	repository := commands.NewMemoryRepository()
	service, err := commands.NewService(repository, deviceService, commands.WithClock(clock), commands.WithLeaseDuration(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := service.Enqueue(context.Background(), commands.EnqueueRequest{
		DeviceID: enrolled.Device.ID, Type: commands.TypeDiagnosticsCollect, Risk: devices.RiskLow,
		Payload: json.RawMessage(`{}`), IdempotencyKey: "diag-001", MaxAttempts: 2,
		ExpiresAt: clock.Now().Add(time.Minute), CreatedBy: "agent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	lease1, err := service.LeaseNext(context.Background(), enrolled.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(10 * time.Second)
	lease2, err := service.LeaseNext(context.Background(), enrolled.Device.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lease1.ID == lease2.ID || lease2.Command.Attempts != 2 {
		t.Fatalf("lease was not safely reclaimed: first=%+v second=%+v", lease1, lease2)
	}
	_, err = service.Complete(context.Background(), command.ID, lease1.ID, commands.Result{Success: true})
	if !commands.IsCode(err, commands.ErrLeaseMismatch) {
		t.Fatalf("stale lease should not complete command: %v", err)
	}
	_, err = service.Complete(context.Background(), command.ID, lease2.ID, commands.Result{Success: true})
	if err != nil {
		t.Fatalf("active lease could not complete command: %v", err)
	}
}

func TestExpiredCommandIsNeverLeased(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 6, 5, 19, 0, 0, 0, time.UTC)}
	policy := devices.Policy{AllowedCommandTypes: []string{string(commands.TypeHealthCheck)}, MaxRisk: devices.RiskLow, ReleaseChannel: devices.ChannelStable}
	deviceService, enrolled := newApprovedDevice(t, clock, policy)
	repository := commands.NewMemoryRepository()
	service, err := commands.NewService(repository, deviceService, commands.WithClock(clock))
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := service.Enqueue(context.Background(), commands.EnqueueRequest{
		DeviceID: enrolled.Device.ID, Type: commands.TypeHealthCheck, Risk: devices.RiskLow,
		Payload: json.RawMessage(`{}`), IdempotencyKey: "expiring", MaxAttempts: 1,
		ExpiresAt: clock.Now().Add(time.Second), CreatedBy: "system",
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	_, err = service.LeaseNext(context.Background(), enrolled.Device.ID)
	if !commands.IsCode(err, commands.ErrCommandNotLeaseable) {
		t.Fatalf("expected no lease for expired command, got %v", err)
	}
	stored, err := repository.Get(context.Background(), command.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != commands.StatusExpired {
		t.Fatalf("expired command stayed in %s", stored.Status)
	}
}
