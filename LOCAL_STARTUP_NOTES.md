# Local Startup Notes

This repo is now configured for local startup without Docker.

## Start

Double-click:

```bat
start-local.cmd
```

Or run:

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\start-local.ps1
```

## Stop

Double-click:

```bat
stop-local.cmd
```

Or run:

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\stop-local.ps1
```

## Current assumptions

The local script now assumes these services already exist on this machine:

- PostgreSQL: `127.0.0.1:5432`
- Redis or Memurai: `127.0.0.1:6379`

The backend binds to `0.0.0.0` by default, so LAN clients can use:

```text
http://192.168.55.32:8080
```

If Windows Firewall blocks inbound access, allow the backend process or port `8080` on private networks.

## First thing to check if backend fails

Open:

```text
.localdev/local-env.ps1
```

Default generated values are:

- `DATABASE_USER=postgres`
- `DATABASE_PASSWORD=postgres`
- `DATABASE_DBNAME=sub2api`
- `DATABASE_SSLMODE=disable`

If your local PostgreSQL password or database name is different, edit that file and run `start-local.cmd` again.

## Generated local files

- `.localdev/local-env.ps1`
- `.localdev/logs/backend.log`
- `.localdev/logs/frontend.log`
- `.localdev/backend-data/config.yaml`

## Optional flags

Skip frontend reinstall:

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\start-local.ps1 -SkipInstall
```

Do not auto-open browser:

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\start-local.ps1 -NoBrowser
```
