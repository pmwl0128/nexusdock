package nexusapp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/uvwt/nexusdock/internal/auth"
	"github.com/uvwt/nexusdock/internal/config"
	"github.com/uvwt/nexusdock/internal/core"
)

func adminCommandRequested(args []string) bool {
	return len(args) >= 2 && args[1] == "admin"
}

func runAdminCommand(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) < 3 {
		return adminUsageError(args)
	}
	command := strings.TrimSpace(args[2])
	if command != "init" && command != "recover" {
		return adminUsageError(args)
	}
	username := ""
	if len(args) > 3 {
		username = strings.TrimSpace(args[3])
	}
	if command == "init" && username == "" {
		return errors.New("username is required")
	}

	controlDir := cfg.NexusDataDir
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return err
	}
	dbPath := filepath.Join(controlDir, "nexus.db")
	db, err := core.OpenSQLite(ctx, dbPath, 1)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := core.EnsureSchema(ctx, db); err != nil {
		return err
	}
	service := auth.NewService(db)
	reader := bufio.NewReader(os.Stdin)
	switch command {
	case "init":
		secret, err := readConfirmedCredential(reader)
		if err != nil {
			return err
		}
		return service.InitializeAdmin(ctx, username, secret)
	case "recover":
		secret, err := readConfirmedCredential(reader)
		if err != nil {
			return err
		}
		return service.RotateAdminCredential(ctx, username, secret)
	}
	return adminUsageError(args)
}

func adminUsageError(args []string) error {
	return fmt.Errorf("usage: %s admin <init|recover> [username]", executableName(args))
}

func executableName(args []string) string {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "nexusdock"
	}
	name := filepath.Base(args[0])
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "nexusdock"
	}
	return name
}

func readConfirmedCredential(reader *bufio.Reader) (string, error) {
	first, err := readHiddenLine(reader, "New credential: ")
	if err != nil {
		return "", err
	}
	second, err := readHiddenLine(reader, "Confirm credential: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("credentials do not match")
	}
	return first, nil
}

func readHiddenLine(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	hide := exec.Command("stty", "-echo")
	hide.Stdin = os.Stdin
	hidden := hide.Run() == nil
	line, err := reader.ReadString('\n')
	if hidden {
		show := exec.Command("stty", "echo")
		show.Stdin = os.Stdin
		_ = show.Run()
		fmt.Fprintln(os.Stderr)
	}
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
