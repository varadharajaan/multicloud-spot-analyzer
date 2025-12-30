# Spot Analyzer - Development Server Startup Script
# Run this script to build and start the web server for frontend testing

Write-Host ""
Write-Host "🚀 Spot Analyzer - Dev Server" -ForegroundColor Cyan
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
Write-Host ""

# Set port (default 8000, can override with parameter)
param(
    [int]$Port = 8000
)

# Navigate to project directory
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptPath

# Build the web server
Write-Host "📦 Building web server..." -ForegroundColor Yellow
go build -o spot-web.exe ./cmd/web

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Build failed!" -ForegroundColor Red
    exit 1
}

Write-Host "✅ Build successful!" -ForegroundColor Green
Write-Host ""

# Start the server
Write-Host "🌐 Starting server on http://localhost:$Port" -ForegroundColor Cyan
Write-Host "   Press Ctrl+C to stop" -ForegroundColor Gray
Write-Host ""

# Open browser automatically
Start-Process "http://localhost:$Port"

# Run the server
.\spot-web.exe --port $Port
