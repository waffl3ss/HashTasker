# HashTasker Update Guide

This guide covers how to update HashTasker server and worker binaries. HashTasker is designed for binary-swap updates — no database changes or migrations are needed. Just replace the binary and restart.

---

## Prerequisites

- Go 1.24+ installed on the build machine
- SUDO/ROOT Access to the server and worker machines (SSH/console)
- The HashTasker source code

---

## 1. Build the Binaries

From the `Server/` directory (which contains both server and worker source):

```bash
./build.sh
```

This produces two binaries:
- `hashtasker-server` — the all-in-one server (web UI, API, database, embedded assets)
- `hashtasker-worker` — the worker that runs hashcat jobs

You can also build them individually:

```bash
# Server only
go build -ldflags="-s -w" -trimpath -o hashtasker-server server.go

# Worker only
go build -ldflags="-s -w" -trimpath -o hashtasker-worker worker.go
```

> **Cross-compilation**: If building on a different OS/architecture than the target, set `GOOS` and `GOARCH`:
> ```bash
> GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o hashtasker-server server.go
> GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -trimpath -o hashtasker-worker worker.go
> ```

---

## 2. Update the Server

### Stop the service

```bash
sudo systemctl stop hashtasker-server
```

### Replace the binary

Copy the new `hashtasker-server` binary to the install directory (default: `/opt/HashTaskerGo/`):

```bash
sudo cp hashtasker-server /opt/HashTaskerGo/hashtasker-server
sudo chmod +x /opt/HashTaskerGo/hashtasker-server
```

### Verify the service file

The systemd service file should be at `/etc/systemd/system/hashtasker-server.service`:

```ini
[Unit]
Description=HashTasker Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/HashTaskerGo
ExecStart=/opt/HashTaskerGo/hashtasker-server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

If the binary path or working directory has changed, update the service file and reload:

```bash
sudo systemctl daemon-reload
```

### Start the service

```bash
sudo systemctl start hashtasker-server
```

### Verify it's running

```bash
sudo systemctl status hashtasker-server
sudo journalctl -u hashtasker-server -f --no-pager -n 50
```

Look for `Database migrations completed successfully` and `Connected to SQLite database successfully` in the logs. Any new database columns or cleanup tasks run automatically on startup.

---

## 3. Update the Workers

Repeat for each worker machine.

### Stop the service

```bash
sudo systemctl stop hashtasker-worker
```

### Replace the binary

```bash
sudo cp hashtasker-worker /opt/HashTaskerGo/hashtasker-worker
sudo chmod +x /opt/HashTaskerGo/hashtasker-worker
```

### Verify the service file

The systemd service file should be at `/etc/systemd/system/hashtasker-worker.service`:

```ini
[Unit]
Description=HashTasker Worker
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/HashTaskerGo
ExecStart=/opt/HashTaskerGo/hashtasker-worker -config worker.json
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

If the binary path or config path changed, update the service file and reload:

```bash
sudo systemctl daemon-reload
```

### Start the service

```bash
sudo systemctl start hashtasker-worker
sudo journalctl -u hashtasker-worker -f --no-pager -n 50
```

---

## 4. Update Order

**Server first, then workers.** The server is always backward-compatible with older workers:

- **New server + old workers**: Works fine. New fields in job assignments are silently ignored by old workers.
- **New workers + old server**: Also works. Missing fields default to safe values (e.g., `username_mode` defaults to `false`).

That said, updating the server first ensures any new database schema changes are applied before workers start sending data that depends on them.

---

## 5. Configuration Files

These files are **not** embedded in the binary and do not need to change between versions (unless noted in the changelog):

### Server — `config.yaml`

Located in the working directory (default: `/opt/HashTaskerGo/config.yaml`).

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  session_key: "change-this-to-a-secure-random-string"
  upload_path: "./uploads"
  static_path: "./web/static"
  template_path: "./web/templates"
  wordlists_path: "./wordlists"
  rulesets_path: "./rules"
  hashmodes_path: "./hashmodes.txt"

database:
  host: "localhost"
  port: 3306
  user: "hashtasker"
  password: "your-secure-password"
  database: "hashtasker"

ldap:
  enabled: false
  # ... ldap settings if needed

admin:
  username: "admin"
  password: "changeme"
```

### Worker — `worker.json`

Located in the working directory on each worker machine (default: `/opt/HashTaskerGo/worker.json`).

```json
{
  "server_url": "http://your-server:8080",
  "hostname": "worker-01",
  "checkin_interval": "30s",
  "hashcat_path": "/usr/bin/hashcat",
  "rulesets_path": "/opt/rules",
  "temp_path": "/tmp/hashtasker"
}
```

---

## 6. Database

HashTasker uses SQLite (`hashtasker.db` in the working directory). The database is **never** replaced during updates. All schema changes are handled automatically by GORM AutoMigrate on startup — new columns are added with safe defaults, and existing data is preserved.

**What happens on first boot after an update:**
- New columns are added to existing tables (if any)
- Retroactive data fixes run (e.g., flagging NTDS jobs, cleaning orphaned records)
- All migration steps are idempotent — safe to run repeatedly

**Backup** (optional but recommended before major updates):

```bash
cp /opt/HashTaskerGo/hashtasker.db /opt/HashTaskerGo/hashtasker.db.bak
```

---

## 7. Quick Reference

```bash
# === BUILD ===
cd Server/
./build.sh

# === SERVER UPDATE ===
sudo systemctl stop hashtasker-server
sudo cp hashtasker-server /opt/HashTaskerGo/hashtasker-server
sudo chmod +x /opt/HashTaskerGo/hashtasker-server
sudo systemctl start hashtasker-server

# === WORKER UPDATE (repeat per worker) ===
sudo systemctl stop hashtasker-worker
sudo cp hashtasker-worker /opt/HashTaskerGo/hashtasker-worker
sudo chmod +x /opt/HashTaskerGo/hashtasker-worker
sudo systemctl start hashtasker-worker

# === VERIFY ===
sudo systemctl status hashtasker-server
sudo systemctl status hashtasker-worker
```

---

## Troubleshooting Updates

| Symptom | Cause | Fix |
|---------|-------|-----|
| Service won't start | Binary not executable | `chmod +x /opt/HashTaskerGo/hashtasker-*` |
| Service starts then stops | Config file missing/invalid | Check `config.yaml` or `worker.json` exists in WorkingDirectory |
| "Failed to migrate database" | Corrupted DB (rare) | Restore from backup: `cp hashtasker.db.bak hashtasker.db` |
| Workers not picking up jobs | Server not reachable | Verify `server_url` in `worker.json`, check firewall |
| Old UI showing after update | Browser cache | Hard refresh: `Ctrl+Shift+R` |
