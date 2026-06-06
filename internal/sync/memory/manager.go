// Package memorysync preserves MemoryDock's Git synchronization behind the
// AgentDock Nexus module path. It is a compatibility facade, not a second sync
// implementation.
package memorysync

import (
	"log/slog"

	"github.com/uvwt/memorydock/internal/syncer"
)

type Config = syncer.Config
type Status = syncer.Status
type ChangedFile = syncer.ChangedFile
type Diff = syncer.Diff
type Commit = syncer.Commit
type Log = syncer.Log
type CommitFile = syncer.CommitFile
type CommitDetail = syncer.CommitDetail
type Manager = syncer.Manager

func NewManager(cfg Config, logger *slog.Logger) *Manager {
	return syncer.NewManager(cfg, logger)
}
