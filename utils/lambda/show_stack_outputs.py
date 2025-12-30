#!/usr/bin/env python3
"""
Show SAM/CloudFormation stack outputs in a nicely formatted way.
Saves to stack-outputs.txt and prints to console.

Usage:
    python utils/lambda/show_stack_outputs.py
    python utils/lambda/show_stack_outputs.py --stack spot-analyzer-stack
    python utils/lambda/show_stack_outputs.py --region us-west-2
"""

import boto3
import json
import argparse
from datetime import datetime
import os

# Project root directory
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# Configuration
DEFAULT_STACK_NAME = "spot-analyzer-stack"
DEFAULT_REGION = "us-east-1"
OUTPUT_FILE = "stack-outputs.txt"

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


def get_stack_outputs(stack_name: str, region: str):
    """Fetch outputs from CloudFormation stack."""
    cf = boto3.client("cloudformation", region_name=region)
    
    try:
        response = cf.describe_stacks(StackName=stack_name)
        stacks = response.get("Stacks", [])
        
        if not stacks:
            print(f"{Colors.RED}❌ Stack '{stack_name}' not found{Colors.RESET}")
            return None
        
        stack = stacks[0]
        status = stack.get("StackStatus", "UNKNOWN")
        
        print(f"{Colors.CYAN}📦 Stack Status: {status}{Colors.RESET}")
        
        return stack.get("Outputs", [])
    except Exception as e:
        print(f"{Colors.RED}❌ Error fetching stack: {e}{Colors.RESET}")
        return None


def format_outputs(outputs, stack_name: str, region: str):
    """Format outputs into a nice string."""
    line = "=" * 80
    
    content = f"""{line}
                    SPOT ANALYZER STACK DEPLOYMENT OUTPUTS
{line}
Fetched:  {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
Stack:    {stack_name}
Region:   {region}
{line}

"""
    
    # Sort by key name
    for output in sorted(outputs, key=lambda x: x.get("OutputKey", "")):
        key = output.get("OutputKey", "Unknown")
        desc = output.get("Description", "No description")
        value = output.get("OutputValue", "N/A")
        
        content += f"{key}\n  {desc}\n  {value}\n\n"
    
    content += line
    return content


def print_outputs(outputs, stack_name: str, region: str):
    """Print outputs with colors."""
    line = "=" * 80
    
    print(f"\n{Colors.CYAN}{line}{Colors.RESET}")
    print(f"{Colors.BOLD}                    SPOT ANALYZER STACK OUTPUTS{Colors.RESET}")
    print(f"{Colors.CYAN}{line}{Colors.RESET}")
    print(f"{Colors.BLUE}Stack:  {stack_name}{Colors.RESET}")
    print(f"{Colors.BLUE}Region: {region}{Colors.RESET}")
    print(f"{Colors.CYAN}{line}{Colors.RESET}\n")
    
    for output in sorted(outputs, key=lambda x: x.get("OutputKey", "")):
        key = output.get("OutputKey", "Unknown")
        desc = output.get("Description", "No description")
        value = output.get("OutputValue", "N/A")
        
        print(f"{Colors.BOLD}{key}{Colors.RESET}")
        print(f"  {Colors.BLUE}{desc}{Colors.RESET}")
        
        # Highlight URLs
        if "http" in value.lower():
            print(f"  {Colors.GREEN}🔗 {value}{Colors.RESET}\n")
        else:
            print(f"  {value}\n")
    
    print(f"{Colors.CYAN}{line}{Colors.RESET}")


def main():
    parser = argparse.ArgumentParser(
        description="Show SAM/CloudFormation stack outputs",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    python show_stack_outputs.py
    python show_stack_outputs.py --stack spot-analyzer-stack
    python show_stack_outputs.py --region us-west-2
    python show_stack_outputs.py --json
        """
    )
    parser.add_argument(
        "--stack", "-s",
        default=DEFAULT_STACK_NAME,
        help=f"CloudFormation stack name (default: {DEFAULT_STACK_NAME})"
    )
    parser.add_argument(
        "--region", "-r",
        default=DEFAULT_REGION,
        help=f"AWS region (default: {DEFAULT_REGION})"
    )
    parser.add_argument(
        "--json", "-j",
        action="store_true",
        help="Output as JSON"
    )
    parser.add_argument(
        "--no-save",
        action="store_true",
        help="Don't save to file"
    )
    
    args = parser.parse_args()
    
    print(f"{Colors.CYAN}📦 Fetching outputs from stack: {args.stack}...{Colors.RESET}")
    
    outputs = get_stack_outputs(args.stack, args.region)
    if not outputs:
        print(f"{Colors.YELLOW}⚠️  No outputs found for stack '{args.stack}'{Colors.RESET}")
        return
    
    # JSON output
    if args.json:
        output_dict = {
            output.get("OutputKey"): output.get("OutputValue")
            for output in outputs
        }
        print(json.dumps(output_dict, indent=2))
        return
    
    # Pretty print
    print_outputs(outputs, args.stack, args.region)
    
    # Save to file
    if not args.no_save:
        formatted = format_outputs(outputs, args.stack, args.region)
        output_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), OUTPUT_FILE)
        
        with open(output_path, "w", encoding="utf-8") as f:
            f.write(formatted)
        
        print(f"\n{Colors.GREEN}✅ Saved to {OUTPUT_FILE}{Colors.RESET}")


if __name__ == "__main__":
    main()
