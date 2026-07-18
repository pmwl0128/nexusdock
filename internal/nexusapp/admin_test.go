package nexusapp

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/config"
)

func TestAdminCommandRequested(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "缺少参数", args: []string{"nexusdock"}},
		{name: "只有 admin", args: []string{"nexusdock", "admin"}, want: true},
		{name: "初始化", args: []string{"nexusdock", "admin", "init", "owner"}, want: true},
		{name: "普通服务参数", args: []string{"nexusdock", "serve"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := adminCommandRequested(tt.args); got != tt.want {
				t.Fatalf("adminCommandRequested(%q) = %v, 期望 %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestInvalidAdminCommandHasNoFilesystemSideEffect(t *testing.T) {
	for _, args := range [][]string{
		{"nexusdock", "admin"},
		{"nexusdock", "admin", "unknown"},
		{"nexusdock", "admin", "init"},
	} {
		args := args
		t.Run(strings.Join(args[1:], "_"), func(t *testing.T) {
			dataDir := filepath.Join(t.TempDir(), "should-not-exist")
			err := runAdminCommand(context.Background(), config.Config{NexusDataDir: dataDir}, args)
			if err == nil {
				t.Fatal("无效管理员命令未返回错误")
			}
			if _, statErr := os.Stat(dataDir); !os.IsNotExist(statErr) {
				t.Fatalf("无效管理员命令创建了数据目录: %v", statErr)
			}
		})
	}
}

func TestAdminUsageUsesExecutableName(t *testing.T) {
	err := adminUsageError([]string{"/usr/local/bin/nexusdock", "admin"})
	if err == nil || err.Error() != "usage: nexusdock admin <init|recover> [username]" {
		t.Fatalf("管理员命令用法错误 = %v", err)
	}
}

func TestReadConfirmedCredential(t *testing.T) {
	credential, err := readConfirmedCredential(bufio.NewReader(strings.NewReader("correct horse battery staple\ncorrect horse battery staple\n")))
	if err != nil {
		t.Fatalf("读取确认凭据失败: %v", err)
	}
	if credential != "correct horse battery staple" {
		t.Fatalf("凭据 = %q", credential)
	}

	if _, err := readConfirmedCredential(bufio.NewReader(strings.NewReader("first value\nsecond value\n"))); err == nil {
		t.Fatal("两次凭据不一致时未返回错误")
	}
}
