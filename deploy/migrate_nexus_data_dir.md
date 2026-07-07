# Migrate Nexus Data Out Of Recall Repository

## Preflight

1. Stop Nexus writes.
2. Back up the Recall repository.
3. Back up `recall/.nexus/control-plane.db`, WAL, SHM, and `recall/.nexus/artifacts`.
4. Choose `NEXUS_DATA_DIR`.

## Migration

```bash
mkdir -p "$NEXUS_DATA_DIR"
cp -a "$RECALL_REPO_DIR/.nexus/control-plane.db"* "$NEXUS_DATA_DIR"/
cp -a "$RECALL_REPO_DIR/.nexus/artifacts" "$NEXUS_DATA_DIR"/
if [ -f "$NEXUS_DATA_DIR/control-plane.db" ]; then
  mv "$NEXUS_DATA_DIR/control-plane.db" "$NEXUS_DATA_DIR/nexus.db"
fi
```

## Verification

```bash
sqlite3 "$NEXUS_DATA_DIR/nexus.db" 'PRAGMA quick_check;'
curl -fsS http://127.0.0.1:18777/health
curl -fsS http://127.0.0.1:18777/v1/system/status
```

## Rollback

Stop Nexus, restore the previous image and previous data directory, then restart. Do not run two Nexus instances against the same SQLite files.
