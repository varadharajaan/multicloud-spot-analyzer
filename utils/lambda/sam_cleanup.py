#!/usr/bin/env python3
"""
SAM Stack Cleanup Script for Spot Analyzer
-------------------------------------------
Deletes the SAM stack and all associated resources.

Usage:
    python utils/lambda/sam_cleanup.py                    # Cleanup default (prod) stack
    python utils/lambda/sam_cleanup.py --env dev          # Cleanup dev stack
    python utils/lambda/sam_cleanup.py --stack-name NAME  # Cleanup specific stack
    python utils/lambda/sam_cleanup.py --all              # Cleanup all spot-analyzer stacks
"""

import subprocess
import sys
import os
import time
import argparse
from datetime import datetime

# Change to project root directory
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.chdir(PROJECT_ROOT)

# Configuration
DEFAULT_STACK_NAME = "spot-analyzer-prod"
REGION = "us-east-1"

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


def print_banner(text: str):
    """Print a styled banner."""
    line = "=" * 70
    print(f"\n{Colors.CYAN}{line}{Colors.RESET}")
    print(f"{Colors.BOLD}{Colors.CYAN}  {text}{Colors.RESET}")
    print(f"{Colors.CYAN}{line}{Colors.RESET}\n")


def print_step(step: str, description: str):
    """Print a step indicator."""
    print(f"{Colors.YELLOW}> [{step}]{Colors.RESET} {description}")


def print_success(message: str):
    """Print success message."""
    print(f"{Colors.GREEN}✓ {message}{Colors.RESET}")


def print_error(message: str):
    """Print error message."""
    print(f"{Colors.RED}✗ {message}{Colors.RESET}")


def print_info(message: str):
    """Print info message."""
    print(f"{Colors.BLUE}ℹ {message}{Colors.RESET}")


def print_warning(message: str):
    """Print warning message."""
    print(f"{Colors.YELLOW}⚠ {message}{Colors.RESET}")


def run_aws_command(cmd: list, description: str, check: bool = True) -> tuple[bool, str]:
    """Run an AWS CLI command and return success status and output."""
    print_step("RUN", description)
    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            shell=True if os.name == 'nt' else False
        )
        if result.returncode == 0:
            return True, result.stdout.strip()
        else:
            if check:
                return False, result.stderr.strip()
            return True, result.stderr.strip()
    except Exception as e:
        return False, str(e)


def get_stack_status(stack_name: str, region: str) -> str | None:
    """Get the current status of a CloudFormation stack."""
    cmd = [
        "aws", "cloudformation", "describe-stacks",
        "--stack-name", stack_name,
        "--region", region,
        "--query", "Stacks[0].StackStatus",
        "--output", "text"
    ]
    success, output = run_aws_command(cmd, f"Checking stack status: {stack_name}", check=False)
    if success and "does not exist" not in output.lower() and output:
        return output
    return None


def list_spot_analyzer_stacks(region: str) -> list[str]:
    """List all spot-analyzer stacks in the region."""
    cmd = [
        "aws", "cloudformation", "list-stacks",
        "--region", region,
        "--stack-status-filter", 
        "CREATE_COMPLETE", "UPDATE_COMPLETE", "ROLLBACK_COMPLETE",
        "UPDATE_ROLLBACK_COMPLETE", "CREATE_FAILED", "DELETE_FAILED",
        "--query", "StackSummaries[?contains(StackName, 'spot-analyzer')].StackName",
        "--output", "text"
    ]
    success, output = run_aws_command(cmd, "Listing spot-analyzer stacks", check=False)
    if success and output:
        return output.split()
    return []


def delete_log_group(function_name: str, region: str) -> bool:
    """Delete the CloudWatch Log Group for a Lambda function."""
    log_group_name = f"/aws/lambda/{function_name}"
    cmd = [
        "aws", "logs", "delete-log-group",
        "--log-group-name", log_group_name,
        "--region", region
    ]
    success, output = run_aws_command(cmd, f"Deleting log group: {log_group_name}", check=False)
    if success or "ResourceNotFoundException" in output:
        return True
    return False


def delete_stack(stack_name: str, region: str) -> bool:
    """Delete a CloudFormation stack."""
    # Check if stack exists
    status = get_stack_status(stack_name, region)
    if not status:
        print_warning(f"Stack '{stack_name}' does not exist or already deleted")
        return True
    
    print_info(f"Stack status: {status}")
    
    # Delete the stack
    cmd = [
        "aws", "cloudformation", "delete-stack",
        "--stack-name", stack_name,
        "--region", region
    ]
    success, output = run_aws_command(cmd, f"Initiating stack deletion: {stack_name}")
    if not success:
        print_error(f"Failed to delete stack: {output}")
        return False
    
    print_success("Delete initiated")
    
    # Wait for deletion to complete
    print_step("WAIT", "Waiting for stack deletion to complete...")
    cmd_wait = [
        "aws", "cloudformation", "wait", "stack-delete-complete",
        "--stack-name", stack_name,
        "--region", region
    ]
    
    start_time = time.time()
    try:
        result = subprocess.run(
            cmd_wait,
            capture_output=True,
            text=True,
            timeout=300,  # 5 minute timeout
            shell=True if os.name == 'nt' else False
        )
        elapsed = time.time() - start_time
        
        if result.returncode == 0:
            print_success(f"Stack deleted successfully in {elapsed:.1f}s")
            return True
        else:
            # Check if it's because stack doesn't exist (already deleted)
            status = get_stack_status(stack_name, region)
            if not status:
                print_success(f"Stack deleted successfully in {elapsed:.1f}s")
                return True
            else:
                print_error(f"Stack deletion failed. Current status: {status}")
                return False
                
    except subprocess.TimeoutExpired:
        print_error("Timeout waiting for stack deletion")
        return False


def cleanup_orphaned_resources(env: str, region: str):
    """Clean up any orphaned resources that might remain after stack deletion."""
    function_name = f"spot-analyzer-{env}"
    
    print_step("CLEANUP", "Checking for orphaned resources...")
    
    # Delete orphaned log group
    delete_log_group(function_name, region)
    
    # Check for orphaned Lambda function (shouldn't exist if stack deleted properly)
    cmd = [
        "aws", "lambda", "get-function",
        "--function-name", function_name,
        "--region", region
    ]
    success, output = run_aws_command(cmd, f"Checking for orphaned Lambda: {function_name}", check=False)
    
    if success and "ResourceNotFoundException" not in output:
        print_warning(f"Found orphaned Lambda function: {function_name}")
        cmd_delete = [
            "aws", "lambda", "delete-function",
            "--function-name", function_name,
            "--region", region
        ]
        success, _ = run_aws_command(cmd_delete, f"Deleting orphaned Lambda: {function_name}", check=False)
        if success:
            print_success("Orphaned Lambda deleted")


def main():
    parser = argparse.ArgumentParser(
        description="SAM Stack Cleanup Script for Spot Analyzer",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    python utils/lambda/sam_cleanup.py                    # Cleanup prod stack
    python utils/lambda/sam_cleanup.py --env dev          # Cleanup dev stack
    python utils/lambda/sam_cleanup.py --stack-name NAME  # Cleanup specific stack
    python utils/lambda/sam_cleanup.py --all              # Cleanup all spot-analyzer stacks
    python utils/lambda/sam_cleanup.py --dry-run          # Show what would be deleted
        """
    )
    parser.add_argument("--env", default="prod", choices=["dev", "prod"], 
                        help="Environment to cleanup (default: prod)")
    parser.add_argument("--stack-name", help="Specific stack name to delete")
    parser.add_argument("--region", default=REGION, help="AWS region")
    parser.add_argument("--all", action="store_true", help="Delete all spot-analyzer stacks")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be deleted without deleting")
    parser.add_argument("-y", "--yes", action="store_true", help="Skip confirmation prompt")
    
    args = parser.parse_args()
    
    region = args.region
    
    print_banner("SPOT ANALYZER STACK CLEANUP")
    print_info(f"Region: {region}")
    print_info(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Determine which stacks to delete
    stacks_to_delete = []
    
    if args.all:
        stacks_to_delete = list_spot_analyzer_stacks(region)
        if not stacks_to_delete:
            print_info("No spot-analyzer stacks found")
            return
    elif args.stack_name:
        stacks_to_delete = [args.stack_name]
    else:
        stacks_to_delete = [f"spot-analyzer-{args.env}"]
    
    print_info(f"Stacks to delete: {', '.join(stacks_to_delete)}")
    
    if args.dry_run:
        print_warning("DRY RUN - No resources will be deleted")
        for stack in stacks_to_delete:
            status = get_stack_status(stack, region)
            if status:
                print_info(f"  Would delete: {stack} (Status: {status})")
            else:
                print_info(f"  Stack not found: {stack}")
        return
    
    # Confirmation prompt
    if not args.yes:
        print()
        print_warning("This will permanently delete the following resources:")
        for stack in stacks_to_delete:
            print(f"  - Stack: {stack}")
            print(f"  - Lambda: spot-analyzer-{args.env}")
            print(f"  - Log Group: /aws/lambda/spot-analyzer-{args.env}")
            print(f"  - IAM Role: Associated execution role")
        print()
        response = input(f"{Colors.YELLOW}Are you sure you want to proceed? (yes/no): {Colors.RESET}")
        if response.lower() not in ['yes', 'y']:
            print_info("Cleanup cancelled")
            return
    
    # Delete stacks
    overall_start = time.time()
    success_count = 0
    fail_count = 0
    
    for stack_name in stacks_to_delete:
        print_banner(f"DELETING STACK: {stack_name}")
        
        if delete_stack(stack_name, region):
            success_count += 1
            # Cleanup orphaned resources
            env = "prod" if "prod" in stack_name else "dev"
            cleanup_orphaned_resources(env, region)
        else:
            fail_count += 1
    
    # Summary
    total_time = time.time() - overall_start
    print_banner("CLEANUP COMPLETE")
    print_success(f"Total time: {total_time:.1f}s")
    print_info(f"Stacks deleted: {success_count}")
    if fail_count > 0:
        print_error(f"Stacks failed: {fail_count}")
        sys.exit(1)


if __name__ == "__main__":
    main()
