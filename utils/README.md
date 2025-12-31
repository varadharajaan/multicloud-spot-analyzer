# Development Utilities

This folder contains development utilities for managing the Spot Analyzer project.

## verify_spot_data - Data Verification Tool

A Go-based tool to verify that the Spot Analyzer is using real AWS Spot Advisor data. Directly queries the AWS Spot Advisor API and displays instance data for specified families.

### Usage

```bash
# Build (one time)
cd utils
go build -o verify_spot_data.exe verify_spot_data.go

# Run with comma-separated families
.\verify_spot_data.exe -families m,c,r

# Show help
.\verify_spot_data.exe
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `-families` | Comma-separated list of instance families (e.g., m,c,r,t) | **Required** |
| `-region` | AWS region | us-east-1 |
| `-os` | Operating system (Linux or Windows) | Linux |
| `-top` | Number of instances to show per family | 20 |
| `-min-savings` | Minimum savings percentage | 0 |
| `-max-interruption` | Maximum interruption level (0-4) | 4 |
| `-json` | Output as JSON | false |

### Examples

```bash
# Verify M-family instances
.\verify_spot_data.exe -families m

# Verify multiple families with filtering
.\verify_spot_data.exe -families m,c,r -min-savings 50 -max-interruption 1

# Output as JSON for scripting
.\verify_spot_data.exe -families t -json

# Check a different region
.\verify_spot_data.exe -families m -region eu-west-1
```

### Interruption Levels

| Level | Range |
|-------|-------|
| 0 | <5% |
| 1 | 5-10% |
| 2 | 10-15% |
| 3 | 15-20% |
| 4 | >20% |

---

## devctl - Development Controller

A comprehensive CLI tool for managing the development environment.

### Quick Start

**Windows (PowerShell):**
```powershell
.\utils\dev\devctl.ps1 start
```

**Windows (CMD):**
```cmd
utils\dev\devctl.cmd start
```

**Linux/macOS:**
```bash
./utils/dev/devctl.sh start
```

### Commands

| Command | Description |
|---------|-------------|
| `start` | Build and start the web server |
| `stop` | Gracefully stop the server |
| `kill` | Force kill the server (and orphan processes) |
| `restart` | Stop and restart the server |
| `status` | Show server status and recent log files |
| `logs` | View or tail log files |
| `build` | Build the Go project |
| `clean` | Clean build artifacts and old logs |

### Examples

```bash
# Start server on default port (8000)
devctl start

# Start server on custom port without opening browser
devctl start -p 3000 --no-browser

# Start without rebuilding
devctl start --no-build

# Stop gracefully
devctl stop

# Force kill (useful if graceful stop fails)
devctl kill

# Restart with rebuild
devctl restart

# Check server status
devctl status

# View last 50 log entries
devctl logs

# View last 100 log entries
devctl logs -n 100

# Tail logs in real-time
devctl logs -t

# View logs from last 2 hours
devctl logs -H 2

# Filter by log level
devctl logs -l error

# Filter by component
devctl logs -c analyzer

# View raw JSON logs
devctl logs --raw

# Build only
devctl build

# Clean everything including logs older than 7 days
devctl clean --logs-days 7
```

### Server Management

The controller manages the server lifecycle:

1. **PID Tracking**: Stores server PID in `.server.pid` for reliable stop/kill
2. **Orphan Detection**: Finds and kills orphan `spot-web` processes
3. **Graceful Shutdown**: Tries SIGTERM before SIGKILL
4. **Port Configuration**: Easily switch ports with `-p` flag

### Log Viewing

The `logs` command supports:

- **Tail mode** (`-t`): Real-time log streaming
- **Time filter** (`-H`): View logs from last N hours
- **Level filter** (`-l`): debug, info, warn, error
- **Component filter** (`-c`): web, cli, analyzer, provider
- **Raw mode** (`--raw`): Show raw JSON lines

### Requirements

- Python 3.7+
- Go 1.21+ (for building)
