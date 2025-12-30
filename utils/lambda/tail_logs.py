#!/usr/bin/env python3
"""
Tail AWS Lambda logs in real-time.

Usage:
    python utils/lambda/tail_logs.py                     # Spot Analyzer logs (default)
    python utils/lambda/tail_logs.py --since 30m         # Last 30 minutes
    python utils/lambda/tail_logs.py --since 1h          # Last 1 hour
    python utils/lambda/tail_logs.py --no-follow         # Don't follow, just show recent
    python utils/lambda/tail_logs.py --env dev           # Dev environment logs
"""
import boto3
import argparse
import time
import re
from datetime import datetime, timedelta, timezone

# Configuration
DEFAULT_REGION = "us-east-1"
DEFAULT_STACK_NAME = "spot-analyzer"
DEFAULT_ENV = "prod"

# Lambda log group pattern
LOG_GROUP_PATTERN = "/aws/lambda/spot-analyzer-{env}"

# Colors for console output
class Colors:
    HEADER = '\033[95m'
    BLUE = '\033[94m'
    CYAN = '\033[96m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    BOLD = '\033[1m'
    RESET = '\033[0m'
    GRAY = '\033[90m'


def parse_since(since_str: str) -> int:
    """Parse since string (e.g., '30m', '1h', '2d') to milliseconds timestamp."""
    match = re.match(r'(\d+)([mhd])', since_str.lower())
    if not match:
        return int((datetime.now(timezone.utc) - timedelta(minutes=5)).timestamp() * 1000)
    
    value, unit = int(match.group(1)), match.group(2)
    if unit == 'm':
        delta = timedelta(minutes=value)
    elif unit == 'h':
        delta = timedelta(hours=value)
    elif unit == 'd':
        delta = timedelta(days=value)
    else:
        delta = timedelta(minutes=5)
    
    return int((datetime.now(timezone.utc) - delta).timestamp() * 1000)


def format_log_event(event: dict, log_group: str) -> str:
    """Format a log event for display."""
    timestamp = datetime.fromtimestamp(event['timestamp'] / 1000)
    message = event['message'].strip()
    
    # Extract lambda name from log group
    lambda_name = log_group.split('/')[-1].replace('spot-analyzer-', '')
    
    prefix = f"[{timestamp.strftime('%H:%M:%S')}] [{lambda_name}]"
    
    # Skip START, END, REPORT lines for cleaner output
    if message.startswith(('START ', 'END ')):
        return None
    
    # Format REPORT lines specially
    if message.startswith('REPORT '):
        # Extract key metrics
        duration_match = re.search(r'Duration: ([\d.]+) ms', message)
        memory_match = re.search(r'Max Memory Used: (\d+) MB', message)
        init_match = re.search(r'Init Duration: ([\d.]+) ms', message)
        
        parts = []
        if duration_match:
            parts.append(f"Duration: {float(duration_match.group(1)):.0f}ms")
        if memory_match:
            parts.append(f"Memory: {memory_match.group(1)}MB")
        if init_match:
            parts.append(f"(Cold Start: {float(init_match.group(1)):.0f}ms)")
        
        if parts:
            return f"{Colors.GRAY}{prefix} 📊 {' | '.join(parts)}{Colors.RESET}"
        return None
    
    # Highlight errors
    if 'ERROR' in message.upper() or 'error' in message.lower() or 'exception' in message.lower():
        return f"{Colors.RED}{prefix} ❌ {message}{Colors.RESET}"
    elif 'WARNING' in message.upper() or 'WARN' in message.upper():
        return f"{Colors.YELLOW}{prefix} ⚠️  {message}{Colors.RESET}"
    elif 'INFO' in message.upper():
        # Clean up INFO prefix if present
        clean_msg = re.sub(r'\[INFO\]\s*', '', message)
        return f"{Colors.GREEN}{prefix} ℹ️  {clean_msg}{Colors.RESET}"
    elif 'DEBUG' in message.upper():
        clean_msg = re.sub(r'\[DEBUG\]\s*', '', message)
        return f"{Colors.GRAY}{prefix} 🔍 {clean_msg}{Colors.RESET}"
    
    # API requests
    if message.startswith('[') and '] POST ' in message or '] GET ' in message:
        return f"{Colors.CYAN}{prefix} 🌐 {message}{Colors.RESET}"
    
    return f"{prefix} {message}"


def tail_logs(log_group: str, region: str, since: str = '5m', follow: bool = True):
    """Tail logs from specified log group."""
    client = boto3.client('logs', region_name=region)
    start_time = parse_since(since)
    
    print(f"{Colors.CYAN}📋 Tailing logs from: {log_group}{Colors.RESET}")
    print(f"{Colors.BLUE}⏰ Since: {since} ago{Colors.RESET}")
    print(f"{Colors.BLUE}🌍 Region: {region}{Colors.RESET}")
    if follow:
        print(f"{Colors.GREEN}🔄 Following... (Ctrl+C to stop){Colors.RESET}")
    print("-" * 70)
    
    seen_events = set()
    last_event_time = start_time
    
    try:
        while True:
            try:
                response = client.filter_log_events(
                    logGroupName=log_group,
                    startTime=start_time,
                    limit=100,
                    interleaved=True
                )
                
                for event in response.get('events', []):
                    event_id = event['eventId']
                    if event_id not in seen_events:
                        seen_events.add(event_id)
                        formatted = format_log_event(event, log_group)
                        if formatted:
                            print(formatted)
                        # Update start time to avoid re-fetching
                        last_event_time = max(last_event_time, event['timestamp'] + 1)
                        start_time = last_event_time
            
            except client.exceptions.ResourceNotFoundException:
                print(f"{Colors.YELLOW}⚠️  Log group not found: {log_group}{Colors.RESET}")
                print(f"{Colors.BLUE}ℹ️  The Lambda function may not have been invoked yet.{Colors.RESET}")
                if not follow:
                    break
            except Exception as e:
                print(f"{Colors.RED}❌ Error fetching logs: {e}{Colors.RESET}")
            
            if not follow:
                break
            
            time.sleep(2)  # Poll every 2 seconds
    
    except KeyboardInterrupt:
        print(f"\n\n{Colors.GREEN}✅ Stopped tailing logs.{Colors.RESET}")


def list_log_groups(region: str, prefix: str = "/aws/lambda/spot-analyzer"):
    """List available log groups matching prefix."""
    client = boto3.client('logs', region_name=region)
    
    try:
        response = client.describe_log_groups(logGroupNamePrefix=prefix)
        groups = response.get('logGroups', [])
        
        if not groups:
            print(f"{Colors.YELLOW}⚠️  No log groups found matching: {prefix}{Colors.RESET}")
            return []
        
        print(f"{Colors.CYAN}📂 Available log groups:{Colors.RESET}")
        for group in groups:
            name = group['logGroupName']
            size_mb = group.get('storedBytes', 0) / (1024 * 1024)
            print(f"  {Colors.GREEN}•{Colors.RESET} {name} ({size_mb:.2f} MB)")
        
        return [g['logGroupName'] for g in groups]
    except Exception as e:
        print(f"{Colors.RED}❌ Error listing log groups: {e}{Colors.RESET}")
        return []


def main():
    parser = argparse.ArgumentParser(
        description='Tail AWS Lambda logs in real-time for Spot Analyzer',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    python tail_logs.py                     # Spot Analyzer prod logs
    python tail_logs.py --env dev           # Dev environment logs
    python tail_logs.py --since 30m         # Last 30 minutes
    python tail_logs.py --since 1h          # Last 1 hour
    python tail_logs.py --no-follow         # Don't follow, just show recent
    python tail_logs.py --list              # List available log groups
        """
    )
    
    parser.add_argument(
        '--env', '-e',
        default=DEFAULT_ENV,
        help=f'Environment (default: {DEFAULT_ENV})'
    )
    parser.add_argument(
        '--region', '-r',
        default=DEFAULT_REGION,
        help=f'AWS region (default: {DEFAULT_REGION})'
    )
    parser.add_argument(
        '--since', '-s',
        default='5m',
        help='How far back to start (e.g., 5m, 30m, 1h, 2d). Default: 5m'
    )
    parser.add_argument(
        '--no-follow', '-n',
        action='store_true',
        help="Don't follow logs, just show recent entries"
    )
    parser.add_argument(
        '--list', '-l',
        action='store_true',
        help='List available log groups'
    )
    parser.add_argument(
        '--log-group', '-g',
        help='Specify exact log group name (overrides --env)'
    )
    
    args = parser.parse_args()
    
    # List log groups if requested
    if args.list:
        list_log_groups(args.region)
        return
    
    # Determine log group
    if args.log_group:
        log_group = args.log_group
    else:
        log_group = LOG_GROUP_PATTERN.format(env=args.env)
    
    tail_logs(log_group, args.region, args.since, follow=not args.no_follow)


if __name__ == '__main__':
    main()
