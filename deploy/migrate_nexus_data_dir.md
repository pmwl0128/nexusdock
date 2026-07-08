# Migrate Nexus Data Out Of Recall Repository

## Preflight

1. Stop Nexus writes with `docker compose stop nexus` or the currently deployed compatibility service name.
2. Back up the full Recall repository.
3. Back up `RECALL_REPO_DIR/.nexus/control-plane.db`, WAL, SHM, `artifacts/`, and `backups/`.
4. Choose a dedicated `NEXUS_DATA_DIR` outside `RECALL_REPO_DIR`.
5. Ensure no other Nexus process is writing either SQLite database.

## Migration

Use SQLite backup rather than copying a live database file directly. This folds WAL state into a consistent `nexus.db` snapshot.

```bash
mkdir -p "$NEXUS_DATA_DIR"
sqlite3 "$RECALL_REPO_DIR/.nexus/control-plane.db" 'PRAGMA wal_checkpoint(TRUNCATE); PRAGMA quick_check;'
sqlite3 "$RECALL_REPO_DIR/.nexus/control-plane.db" ".backup '$NEXUS_DATA_DIR/nexus.db'"

if [ -d "$RECALL_REPO_DIR/.nexus/artifacts" ]; then
  mkdir -p "$NEXUS_DATA_DIR/artifacts"
  cp -a "$RECALL_REPO_DIR/.nexus/artifacts/." "$NEXUS_DATA_DIR/artifacts/"
fi

if [ -d "$RECALL_REPO_DIR/.nexus/backups" ]; then
  mkdir -p "$NEXUS_DATA_DIR/backups"
  cp -a "$RECALL_REPO_DIR/.nexus/backups/." "$NEXUS_DATA_DIR/backups/"
fi
```

Keep `RECALL_REPO_DIR/.nexus` as rollback evidence until the new deployment has been verified and backed up.

## Verification

```bash
sqlite3 "$NEXUS_DATA_DIR/nexus.db" 'PRAGMA quick_check;'
docker compose up -d nexus
curl -fsS http://127.0.0.1:18777/health
curl -fsS -H "Authorization: Bearer $NEXUS_AUTH_TOKEN" http://127.0.0.1:18777/v1/system/status
```

Expected `/v1/system/status` fields:

```json
{
  "service": "nexus",
  "database": "ok",
  "nexus_data_dir": "/var/lib/nexus",
  "recall_repo_dir": "/recall",
  "artifact_root": "/var/lib/nexus/artifacts"
}
```

## Rollback

Stop Nexus, restore the previous image and previous Compose file, then restore the pre-migration database snapshot. Do not run two Nexus instances against the same SQLite files.
