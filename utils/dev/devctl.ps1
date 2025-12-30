# Spot Analyzer Development Controller - PowerShell Wrapper
# Usage: .\devctl.ps1 <command> [options]
#
# Examples:
#   .\devctl.ps1 start              Start server on default port (8000)
#   .\devctl.ps1 start -p 3000      Start server on port 3000
#   .\devctl.ps1 stop               Stop the server gracefully
#   .\devctl.ps1 kill               Force kill the server
#   .\devctl.ps1 restart            Restart the server
#   .\devctl.ps1 status             Show server status
#   .\devctl.ps1 logs               View last 50 log entries
#   .\devctl.ps1 logs -t            Tail logs in real-time
#   .\devctl.ps1 logs -H 2          View logs from last 2 hours
#   .\devctl.ps1 build              Build the project
#   .\devctl.ps1 clean              Clean build artifacts

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
python "$ScriptDir\devctl.py" $args
