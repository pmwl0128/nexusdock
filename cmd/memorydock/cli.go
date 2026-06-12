package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/uvwt/agentdock-nexus/internal/auth"
	"github.com/uvwt/agentdock-nexus/internal/config"
	"github.com/uvwt/agentdock-nexus/internal/core"
)

func adminCommandRequested() bool {
	return len(os.Args) >= 3 && os.Args[1] == "admin"
}

func runAdminCommand(ctx context.Context, cfg config.Config) error {
	controlDir := filepath.Join(cfg.StoreDir, ".nexus")
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return err
	}
	dbPath := filepath.Join(controlDir, "control-plane.db")
	db, err := core.OpenSQLite(ctx, dbPath, 1)
	if err != nil {
		return err
	}
	defer db.Close()
	migrations := core.NewMigrationRunner(db, core.SQLiteBackupHook{SourcePath: dbPath, Directory: filepath.Join(controlDir, "backups")})
	if err := migrations.Run(ctx); err != nil {
		return err
	}
	service := auth.NewService(db)
	reader := bufio.NewReader(os.Stdin)
	command := os.Args[2]
	username := ""
	if len(os.Args) > 3 {
		username = strings.TrimSpace(os.Args[3])
	}
	switch command {
	case "init":
		if username == "" {
			return errors.New("username is required")
		}
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
	default:
		return errors.New("usage: memorydock admin <init|recover> [username]")
	}
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
