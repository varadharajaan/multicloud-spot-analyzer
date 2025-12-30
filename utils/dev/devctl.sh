#!/bin/bash
# Spot Analyzer Development Controller - Unix Wrapper
# Usage: ./devctl <command> [options]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 "$SCRIPT_DIR/devctl.py" "$@"
