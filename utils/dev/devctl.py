#!/usr/bin/env python3
"""
Spot Analyzer Development Controller (devctl)

A comprehensive CLI tool for managing the Spot Analyzer development environment.

Commands:
    start   - Build and start the web server
    stop    - Gracefully stop the server
    kill    - Force kill the server
    restart - Restart the server
    status  - Show server status
    logs    - View/tail logs
    build   - Build the project
    clean   - Clean build artifacts and old logs
"""

import argparse
import datetime
import json
import os
import platform
import signal
import subprocess
import sys
import time
from pathlib import Path
from typing import Optional, List, Dict, Any

# Configuration
DEFAULT_PORT = 8000
PROJECT_ROOT = Path(__file__).parent.parent.parent.resolve()
LOGS_DIR = PROJECT_ROOT / "logs"
PID_FILE = PROJECT_ROOT / ".server.pid"
WEB_EXE = PROJECT_ROOT / "spot-web.exe" if platform.system() == "Windows" else PROJECT_ROOT / "spot-web"

# ANSI colors
class Colors:
    HEADER = '\033[95m'
    BLUE = '\033[94m'
    CYAN = '\033[96m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    END = '\033[0m'
    BOLD = '\033[1m'

def colorize(text: str, color: str) -> str:
    """Add color to text if terminal supports it."""
    if sys.stdout.isatty():
        return f"{color}{text}{Colors.END}"
    return text

def print_banner():
    """Print the Spot Analyzer banner."""
    banner = """
╔══════════════════════════════════════════════════════════════╗
║          🚀 Spot Analyzer - Development Controller           ║
╚══════════════════════════════════════════════════════════════╝
"""
    print(colorize(banner, Colors.CYAN))

def print_success(msg: str):
    print(colorize(f"✅ {msg}", Colors.GREEN))

def print_error(msg: str):
    print(colorize(f"❌ {msg}", Colors.RED))

def print_warning(msg: str):
    print(colorize(f"⚠️  {msg}", Colors.YELLOW))

def print_info(msg: str):
    print(colorize(f"ℹ️  {msg}", Colors.BLUE))

def get_server_pid() -> Optional[int]:
    """Get the server PID from the PID file."""
    if PID_FILE.exists():
        try:
            pid = int(PID_FILE.read_text().strip())
            # Check if process is still running
            if is_process_running(pid):
                return pid
        except (ValueError, FileNotFoundError):
            pass
    return None

def is_process_running(pid: int) -> bool:
    """Check if a process with the given PID is running."""
    if platform.system() == "Windows":
        try:
            result = subprocess.run(
                ["tasklist", "/FI", f"PID eq {pid}", "/NH"],
                capture_output=True, text=True
            )
            return str(pid) in result.stdout
        except Exception:
            return False
    else:
        try:
            os.kill(pid, 0)
            return True
        except OSError:
            return False

def find_server_processes() -> List[int]:
    """Find all running spot-web processes."""
    pids = []
    if platform.system() == "Windows":
        try:
            result = subprocess.run(
                ["tasklist", "/FI", "IMAGENAME eq spot-web.exe", "/NH", "/FO", "CSV"],
                capture_output=True, text=True
            )
            for line in result.stdout.strip().split('\n'):
                if 'spot-web.exe' in line:
                    parts = line.split(',')
                    if len(parts) >= 2:
                        pid = int(parts[1].strip('"'))
                        pids.append(pid)
        except Exception:
            pass
    else:
        try:
            result = subprocess.run(
                ["pgrep", "-f", "spot-web"],
                capture_output=True, text=True
            )
            pids = [int(p) for p in result.stdout.strip().split('\n') if p]
        except Exception:
            pass
    return pids

def find_pid_by_port(port: int) -> Optional[int]:
    """Find process ID using a specific port."""
    if platform.system() == "Windows":
        try:
            result = subprocess.run(
                ["powershell", "-Command", 
                 f"Get-NetTCPConnection -LocalPort {port} -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -First 1"],
                capture_output=True, text=True
            )
            if result.stdout.strip():
                return int(result.stdout.strip())
        except Exception:
            pass
    else:
        try:
            result = subprocess.run(
                ["lsof", "-ti", f":{port}"],
                capture_output=True, text=True
            )
            if result.stdout.strip():
                return int(result.stdout.strip().split('\n')[0])
        except Exception:
            pass
    return None

def kill_by_port(port: int, force: bool = True) -> bool:
    """Kill process using a specific port."""
    pid = find_pid_by_port(port)
    if pid:
        print_info(f"Found process {pid} on port {port}")
        return kill_process(pid, force=force)
    return False

def kill_process(pid: int, force: bool = False) -> bool:
    """Kill a process by PID."""
    try:
        if platform.system() == "Windows":
            cmd = ["taskkill"]
            if force:
                cmd.append("/F")
            cmd.extend(["/PID", str(pid)])
            subprocess.run(cmd, capture_output=True)
        else:
            sig = signal.SIGKILL if force else signal.SIGTERM
            os.kill(pid, sig)
        return True
    except Exception as e:
        print_error(f"Failed to kill process {pid}: {e}")
        return False

def cmd_build(args) -> int:
    """Build the web server."""
    print_info("Building web server...")
    
    os.chdir(PROJECT_ROOT)
    
    # Build web server
    result = subprocess.run(
        ["go", "build", "-o", str(WEB_EXE), "./cmd/web"],
        capture_output=True, text=True
    )
    
    if result.returncode != 0:
        print_error("Build failed!")
        print(result.stderr)
        return 1
    
    print_success("Build successful!")
    return 0

def cmd_start(args) -> int:
    """Start the web server."""
    port = args.port
    
    # Check if already running
    pid = get_server_pid()
    if pid:
        print_warning(f"Server already running (PID: {pid})")
        print_info(f"Use 'devctl restart' to restart or 'devctl stop' to stop")
        return 1
    
    # Build first unless --no-build
    if not args.no_build:
        if cmd_build(args) != 0:
            return 1
    
    if not WEB_EXE.exists():
        print_error(f"Web server executable not found: {WEB_EXE}")
        print_info("Run 'devctl build' first")
        return 1
    
    # Create logs directory
    LOGS_DIR.mkdir(exist_ok=True)
    
    print_info(f"Starting server on http://localhost:{port}")
    
    # Start server in background
    if platform.system() == "Windows":
        # Use CREATE_NEW_PROCESS_GROUP to detach
        process = subprocess.Popen(
            [str(WEB_EXE), "-port", str(port)],
            cwd=PROJECT_ROOT,
            creationflags=subprocess.CREATE_NEW_PROCESS_GROUP | subprocess.DETACHED_PROCESS,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL
        )
    else:
        process = subprocess.Popen(
            [str(WEB_EXE), "-port", str(port)],
            cwd=PROJECT_ROOT,
            start_new_session=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL
        )
    
    # Save PID
    PID_FILE.write_text(str(process.pid))
    
    # Wait a moment for server to start
    time.sleep(1)
    
    if is_process_running(process.pid):
        print_success(f"Server started (PID: {process.pid})")
        print_info(f"🌐 Open: http://localhost:{port}")
        print_info(f"📁 Logs: {LOGS_DIR}")
        
        # Open browser unless --no-browser
        if not args.no_browser:
            open_browser(f"http://localhost:{port}")
        
        return 0
    else:
        print_error("Server failed to start")
        PID_FILE.unlink(missing_ok=True)
        return 1

def cmd_stop(args) -> int:
    """Gracefully stop the server."""
    port = getattr(args, 'port', None)
    
    # If port specified, kill by port
    if port:
        pid = find_pid_by_port(port)
        if not pid:
            print_warning(f"No process found on port {port}")
            return 0
        print_info(f"Stopping server on port {port} (PID: {pid})...")
        if kill_process(pid, force=False):
            for _ in range(10):
                if not is_process_running(pid):
                    break
                time.sleep(0.5)
            if not is_process_running(pid):
                print_success(f"Server on port {port} stopped")
                return 0
            else:
                print_warning("Server didn't stop gracefully, force killing...")
                kill_process(pid, force=True)
                print_success(f"Server on port {port} killed")
                return 0
        return 1
    
    # Otherwise use PID file
    pid = get_server_pid()
    
    if not pid:
        print_warning("Server is not running")
        return 0
    
    print_info(f"Stopping server (PID: {pid})...")
    
    if kill_process(pid, force=False):
        # Wait for graceful shutdown
        for _ in range(10):
            if not is_process_running(pid):
                break
            time.sleep(0.5)
        
        if not is_process_running(pid):
            print_success("Server stopped")
            PID_FILE.unlink(missing_ok=True)
            return 0
        else:
            print_warning("Server didn't stop gracefully, force killing...")
            return cmd_kill(args)
    
    return 1

def cmd_kill(args) -> int:
    """Force kill the server."""
    port = getattr(args, 'port', None)
    killed = False
    
    # If port specified, kill by port
    if port:
        pid = find_pid_by_port(port)
        if pid:
            print_info(f"Force killing process on port {port} (PID: {pid})...")
            if kill_process(pid, force=True):
                killed = True
                print_success(f"Process on port {port} killed")
        else:
            print_warning(f"No process found on port {port}")
        return 0
    
    # Kill from PID file
    pid = get_server_pid()
    
    if pid:
        print_info(f"Force killing server (PID: {pid})...")
        if kill_process(pid, force=True):
            killed = True
    
    # Also find and kill any orphan processes
    pids = find_server_processes()
    for p in pids:
        if p != pid:
            print_info(f"Killing orphan process (PID: {p})...")
            kill_process(p, force=True)
            killed = True
    
    PID_FILE.unlink(missing_ok=True)
    
    if killed:
        print_success("Server killed")
    else:
        print_warning("No server processes found")
    
    return 0

def cmd_restart(args) -> int:
    """Restart the server."""
    print_info("Restarting server...")
    cmd_stop(args)
    time.sleep(1)
    return cmd_start(args)

def cmd_status(args) -> int:
    """Show server status."""
    port = getattr(args, 'port', None)
    
    # If port specified, check that specific port
    if port:
        pid = find_pid_by_port(port)
        if pid:
            print_success(f"Port {port} is in use by process {pid}")
            return 0
        else:
            print_warning(f"Port {port} is free")
            return 0
    
    pid = get_server_pid()
    
    print()
    print(colorize("═══ Server Status ═══", Colors.CYAN))
    print()
    
    if pid:
        print(f"  Status:  {colorize('● Running', Colors.GREEN)}")
        print(f"  PID:     {pid}")
    else:
        pids = find_server_processes()
        if pids:
            print(f"  Status:  {colorize('● Orphan processes', Colors.YELLOW)}")
            print(f"  PIDs:    {pids}")
        else:
            print(f"  Status:  {colorize('○ Stopped', Colors.RED)}")
    
    # Check for log files
    print()
    print(colorize("═══ Log Files ═══", Colors.CYAN))
    print()
    
    if LOGS_DIR.exists():
        # Look for both .log and .jsonl files
        log_files = sorted(list(LOGS_DIR.glob("*.jsonl")) + list(LOGS_DIR.glob("*.log")), 
                          key=lambda f: f.stat().st_mtime, reverse=True)[:10]
        if log_files:
            for lf in log_files:
                size = lf.stat().st_size / 1024  # KB
                mtime = datetime.datetime.fromtimestamp(lf.stat().st_mtime)
                icon = "📊" if lf.suffix == ".jsonl" else "📄"
                print(f"  {icon} {lf.name} ({size:.1f} KB, {mtime:%Y-%m-%d %H:%M})")
        else:
            print("  No log files found")
    else:
        print("  Logs directory doesn't exist")
    
    print()
    return 0

def cmd_logs(args) -> int:
    """View or tail logs."""
    if not LOGS_DIR.exists():
        print_error("Logs directory doesn't exist")
        return 1
    
    # Find log file
    if args.file:
        log_file = LOGS_DIR / args.file
    else:
        # Get latest JSONL file
        log_files = sorted(LOGS_DIR.glob("*.jsonl"), reverse=True)
        if not log_files:
            print_error("No log files found")
            return 1
        log_file = log_files[0]
    
    if not log_file.exists():
        print_error(f"Log file not found: {log_file}")
        return 1
    
    print_info(f"Log file: {log_file.name}")
    print()
    
    if args.tail:
        return tail_logs(log_file, args)
    elif args.hours:
        return view_logs_by_time(log_file, args.hours, args)
    else:
        return view_logs(log_file, args)

def tail_logs(log_file: Path, args) -> int:
    """Tail logs in real-time."""
    print_info("Tailing logs (Ctrl+C to stop)...")
    print()
    
    try:
        with open(log_file, 'r') as f:
            # Go to end of file
            f.seek(0, 2)
            
            while True:
                line = f.readline()
                if line:
                    print_log_entry(line, args)
                else:
                    time.sleep(0.1)
    except KeyboardInterrupt:
        print()
        print_info("Stopped tailing")
    
    return 0

def view_logs_by_time(log_file: Path, hours: float, args) -> int:
    """View logs from the last N hours."""
    cutoff = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(hours=hours)
    
    print_info(f"Showing logs from the last {hours} hour(s)")
    print()
    
    count = 0
    with open(log_file, 'r') as f:
        for line in f:
            try:
                entry = json.loads(line)
                ts = datetime.datetime.fromisoformat(entry.get('timestamp', '').replace('Z', '+00:00'))
                if ts >= cutoff:
                    print_log_entry(line, args)
                    count += 1
            except (json.JSONDecodeError, ValueError):
                continue
    
    print()
    print_info(f"Showed {count} log entries")
    return 0

def view_logs(log_file: Path, args) -> int:
    """View logs with optional filtering."""
    lines = args.lines or 50
    level_filter = args.level.upper() if args.level else None
    component_filter = args.component
    
    # Read all lines and filter
    all_entries = []
    with open(log_file, 'r') as f:
        for line in f:
            try:
                entry = json.loads(line)
                
                # Apply filters
                if level_filter and entry.get('level') != level_filter:
                    continue
                if component_filter and entry.get('component') != component_filter:
                    continue
                
                all_entries.append(line)
            except json.JSONDecodeError:
                continue
    
    # Show last N entries
    for line in all_entries[-lines:]:
        print_log_entry(line, args)
    
    print()
    print_info(f"Showed {min(lines, len(all_entries))} of {len(all_entries)} matching entries")
    return 0

def print_log_entry(line: str, args):
    """Pretty print a log entry."""
    try:
        entry = json.loads(line)
        
        if args.raw:
            print(line.strip())
            return
        
        # Format timestamp
        ts = entry.get('timestamp', '')[:23]  # Truncate nanoseconds
        
        # Color by level
        level = entry.get('level', 'INFO')
        level_colors = {
            'DEBUG': Colors.BLUE,
            'INFO': Colors.GREEN,
            'WARN': Colors.YELLOW,
            'ERROR': Colors.RED,
        }
        level_color = level_colors.get(level, Colors.END)
        level_str = colorize(f"[{level:5}]", level_color)
        
        # Component
        component = entry.get('component', '-')
        component_str = colorize(f"[{component:8}]", Colors.CYAN)
        
        # Message
        message = entry.get('message', '')
        
        # Extra info
        extras = []
        if entry.get('region'):
            extras.append(f"region={entry['region']}")
        if entry.get('instance_type'):
            extras.append(f"instance={entry['instance_type']}")
        if entry.get('duration_ms'):
            extras.append(f"duration={entry['duration_ms']:.1f}ms")
        if entry.get('count'):
            extras.append(f"count={entry['count']}")
        if entry.get('error'):
            extras.append(colorize(f"error={entry['error']}", Colors.RED))
        
        extra_str = " " + " ".join(extras) if extras else ""
        
        print(f"{ts} {level_str} {component_str} {message}{extra_str}")
        
    except json.JSONDecodeError:
        print(line.strip())

def cmd_clean(args) -> int:
    """Clean build artifacts and old logs."""
    print_info("Cleaning up...")
    
    # Remove build artifacts
    artifacts = [WEB_EXE, PROJECT_ROOT / "spot-analyzer.exe", PROJECT_ROOT / "spot-analyzer"]
    for artifact in artifacts:
        if artifact.exists():
            artifact.unlink()
            print(f"  Removed: {artifact.name}")
    
    # Remove old logs
    if args.logs_days and LOGS_DIR.exists():
        cutoff = datetime.datetime.now() - datetime.timedelta(days=args.logs_days)
        for log_file in LOGS_DIR.glob("*"):
            mtime = datetime.datetime.fromtimestamp(log_file.stat().st_mtime)
            if mtime < cutoff:
                log_file.unlink()
                print(f"  Removed old log: {log_file.name}")
    
    # Remove PID file
    PID_FILE.unlink(missing_ok=True)
    
    print_success("Cleanup complete")
    return 0

def open_browser(url: str):
    """Open URL in default browser."""
    try:
        if platform.system() == "Windows":
            os.startfile(url)
        elif platform.system() == "Darwin":
            subprocess.run(["open", url], check=False)
        else:
            subprocess.run(["xdg-open", url], check=False)
    except Exception:
        pass

def main():
    parser = argparse.ArgumentParser(
        description="Spot Analyzer Development Controller",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  devctl start              Start server on default port (8000)
  devctl start -p 3000      Start server on port 3000
  devctl stop               Stop the server gracefully
  devctl stop -p 8080       Stop server on port 8080
  devctl kill               Force kill the server
  devctl kill -p 8080       Force kill process on port 8080
  devctl restart            Restart the server
  devctl restart -p 3000    Restart server on port 3000
  devctl status             Show server status
  devctl status -p 8080     Check if port 8080 is in use
  devctl logs               View last 50 log entries
  devctl logs -t            Tail logs in real-time
  devctl logs -H 2          View logs from last 2 hours
  devctl logs -l error      View only ERROR logs
  devctl logs -c analyzer   View only analyzer component logs
  devctl build              Build the project
  devctl clean              Clean build artifacts
"""
    )
    
    subparsers = parser.add_subparsers(dest='command', help='Commands')
    
    # start command
    start_parser = subparsers.add_parser('start', help='Start the web server')
    start_parser.add_argument('-p', '--port', type=int, default=DEFAULT_PORT, help=f'Port number (default: {DEFAULT_PORT})')
    start_parser.add_argument('--no-build', action='store_true', help='Skip building before starting')
    start_parser.add_argument('--no-browser', action='store_true', help='Do not open browser')
    start_parser.set_defaults(func=cmd_start)
    
    # stop command
    stop_parser = subparsers.add_parser('stop', help='Stop the server gracefully')
    stop_parser.add_argument('-p', '--port', type=int, help='Stop server on specific port')
    stop_parser.set_defaults(func=cmd_stop)
    
    # kill command
    kill_parser = subparsers.add_parser('kill', help='Force kill the server')
    kill_parser.add_argument('-p', '--port', type=int, help='Kill server on specific port')
    kill_parser.set_defaults(func=cmd_kill)
    
    # restart command
    restart_parser = subparsers.add_parser('restart', help='Restart the server')
    restart_parser.add_argument('-p', '--port', type=int, default=DEFAULT_PORT, help=f'Port number (default: {DEFAULT_PORT})')
    restart_parser.add_argument('--no-build', action='store_true', help='Skip building before starting')
    restart_parser.add_argument('--no-browser', action='store_true', help='Do not open browser')
    restart_parser.set_defaults(func=cmd_restart)
    
    # status command
    status_parser = subparsers.add_parser('status', help='Show server status')
    status_parser.add_argument('-p', '--port', type=int, help='Check if specific port is in use')
    status_parser.set_defaults(func=cmd_status)
    
    # logs command
    logs_parser = subparsers.add_parser('logs', help='View or tail logs')
    logs_parser.add_argument('-t', '--tail', action='store_true', help='Tail logs in real-time')
    logs_parser.add_argument('-H', '--hours', type=float, help='View logs from last N hours')
    logs_parser.add_argument('-n', '--lines', type=int, default=50, help='Number of lines to show (default: 50)')
    logs_parser.add_argument('-l', '--level', help='Filter by log level (debug, info, warn, error)')
    logs_parser.add_argument('-c', '--component', help='Filter by component (web, cli, analyzer, provider)')
    logs_parser.add_argument('-f', '--file', help='Specific log file to read')
    logs_parser.add_argument('--raw', action='store_true', help='Show raw JSON output')
    logs_parser.set_defaults(func=cmd_logs)
    
    # build command
    build_parser = subparsers.add_parser('build', help='Build the project')
    build_parser.set_defaults(func=cmd_build)
    
    # clean command
    clean_parser = subparsers.add_parser('clean', help='Clean build artifacts')
    clean_parser.add_argument('--logs-days', type=int, help='Remove logs older than N days')
    clean_parser.set_defaults(func=cmd_clean)
    
    args = parser.parse_args()
    
    if not args.command:
        print_banner()
        parser.print_help()
        return 0
    
    print_banner()
    return args.func(args)

if __name__ == '__main__':
    sys.exit(main())
