#!/usr/bin/env python3
"""
SAM Build & Deploy Script for Spot Analyzer
--------------------------------------------
Builds and deploys the SAM stack with real-time console output.

Usage:
    python utils/lambda/sam_deploy.py           # Build + Deploy (default)
    python utils/lambda/sam_deploy.py -b        # Build only
    python utils/lambda/sam_deploy.py -d        # Deploy only
    python utils/lambda/sam_deploy.py --build   # Build only
    python utils/lambda/sam_deploy.py --deploy  # Deploy only
"""

import subprocess
import sys
import os
import time
import argparse
import shutil
from datetime import datetime

# Change to project root directory
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.chdir(PROJECT_ROOT)

# Configuration
STACK_NAME = "spot-analyzer-stack"
REGION = "us-east-1"  # Change to your preferred region
OUTPUT_FILE = "stack-outputs.txt"
S3_BUCKET = None  # Set to your S3 bucket for deployment, or None for guided

# SAM CLI path (Windows default installation)
SAM_CMD = shutil.which("sam") or r"C:\Program Files\Amazon\AWSSAMCLI\bin\sam.cmd"

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
    print(f"{Colors.GREEN}[OK] {message}{Colors.RESET}")


def print_error(message: str):
    """Print error message."""
    print(f"{Colors.RED}[ERROR] {message}{Colors.RESET}")


def print_info(message: str):
    """Print info message."""
    print(f"{Colors.BLUE}[INFO] {message}{Colors.RESET}")


def run_command(cmd: list, description: str) -> bool:
    """Run a command with real-time output streaming."""
    print_step("RUN", f"{description}")
    print(f"{Colors.CYAN}    Command: {' '.join(cmd)}{Colors.RESET}\n")
    
    start_time = time.time()
    
    # Set up environment
    env = os.environ.copy()
    env["GOOS"] = "linux"
    env["GOARCH"] = "amd64"
    env["CGO_ENABLED"] = "0"
    
    try:
        # Use Popen for real-time output
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
            universal_newlines=True,
            env=env,
            shell=True if os.name == 'nt' else False
        )
        
        # Stream output in real-time
        for line in process.stdout:
            line = line.rstrip()
            if "error" in line.lower() or "failed" in line.lower():
                print(f"    {Colors.RED}{line}{Colors.RESET}")
            elif "warning" in line.lower():
                print(f"    {Colors.YELLOW}{line}{Colors.RESET}")
            elif "successfully" in line.lower() or "complete" in line.lower():
                print(f"    {Colors.GREEN}{line}{Colors.RESET}")
            elif line.startswith("Key") or line.startswith("Value") or line.startswith("Description"):
                print(f"    {Colors.CYAN}{line}{Colors.RESET}")
            else:
                print(f"    {line}")
        
        process.wait()
        duration = time.time() - start_time
        
        if process.returncode == 0:
            print_success(f"{description} completed in {duration:.1f}s")
            return True
        else:
            print_error(f"{description} failed with exit code {process.returncode}")
            return False
            
    except FileNotFoundError:
        print_error(f"Command not found: {cmd[0]}")
        print_info("Make sure AWS SAM CLI is installed: pip install aws-sam-cli")
        return False
    except Exception as e:
        print_error(f"Error running command: {e}")
        return False


def build_go_binary():
    """Build the Go Lambda binary."""
    print_step("BUILD", "Building Go Lambda binary for Linux...")
    
    env = os.environ.copy()
    env["GOOS"] = "linux"
    env["GOARCH"] = "amd64"
    env["CGO_ENABLED"] = "0"
    
    cmd = ["go", "build", "-o", "bootstrap", "-tags", "lambda.norpc", "./cmd/lambda"]
    
    try:
        result = subprocess.run(
            cmd,
            env=env,
            capture_output=True,
            text=True,
            cwd=PROJECT_ROOT
        )
        
        if result.returncode == 0:
            print_success("Go binary built successfully")
            return True
        else:
            print_error(f"Go build failed: {result.stderr}")
            return False
    except Exception as e:
        print_error(f"Go build error: {e}")
        return False


def save_stack_outputs():
    """Fetch and save stack outputs after successful deploy."""
    print_step("SAVE", "Fetching stack outputs...")
    
    try:
        import boto3
        cf = boto3.client("cloudformation", region_name=REGION)
        response = cf.describe_stacks(StackName=STACK_NAME)
        stacks = response.get("Stacks", [])
        
        if not stacks:
            print_error(f"Stack '{STACK_NAME}' not found")
            return
        
        outputs = stacks[0].get("Outputs", [])
        
        # Format output
        line = "=" * 80
        content = f"""{line}
                    SPOT ANALYZER STACK DEPLOYMENT OUTPUTS
{line}
Deployed: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
Stack:    {STACK_NAME}
Region:   {REGION}
{line}

"""
        
        # Print and save outputs
        print(f"\n{Colors.CYAN}{line}{Colors.RESET}")
        print(f"{Colors.BOLD}                    STACK OUTPUTS{Colors.RESET}")
        print(f"{Colors.CYAN}{line}{Colors.RESET}\n")
        
        for output in sorted(outputs, key=lambda x: x.get("OutputKey", "")):
            key = output.get("OutputKey", "Unknown")
            desc = output.get("Description", "No description")
            value = output.get("OutputValue", "N/A")
            
            content += f"{key}\n  {desc}\n  {value}\n\n"
            
            # Print with colors
            print(f"{Colors.BOLD}{key}{Colors.RESET}")
            print(f"  {Colors.BLUE}{desc}{Colors.RESET}")
            if "http" in value.lower():
                print(f"  {Colors.GREEN}{value}{Colors.RESET}\n")
            else:
                print(f"  {value}\n")
        
        content += line
        
        # Save to file in utils/lambda folder
        output_path = os.path.join(PROJECT_ROOT, "utils", "lambda", OUTPUT_FILE)
        os.makedirs(os.path.dirname(output_path), exist_ok=True)
        
        with open(output_path, "w", encoding="utf-8") as f:
            f.write(content)
        
        print_success(f"Stack outputs saved to utils/lambda/{OUTPUT_FILE}")
        
    except ImportError:
        print_info("boto3 not installed, skipping stack output fetch")
    except Exception as e:
        print_error(f"Failed to fetch stack outputs: {e}")


def main():
    parser = argparse.ArgumentParser(
        description="SAM Build & Deploy Script for Spot Analyzer",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    python utils/lambda/sam_deploy.py           # Build + Deploy
    python utils/lambda/sam_deploy.py -b        # Build only
    python utils/lambda/sam_deploy.py -d        # Deploy only
    python utils/lambda/sam_deploy.py --build   # Build only
    python utils/lambda/sam_deploy.py --deploy  # Deploy only
        """
    )
    parser.add_argument("-b", "--build", action="store_true", help="Build only")
    parser.add_argument("-d", "--deploy", action="store_true", help="Deploy only")
    parser.add_argument("--region", default=REGION, help="AWS region")
    parser.add_argument("--stack-name", default=STACK_NAME, help="CloudFormation stack name")
    
    args = parser.parse_args()
    
    global REGION, STACK_NAME
    REGION = args.region
    STACK_NAME = args.stack_name
    
    # Default: both build and deploy
    do_build = True
    do_deploy = True
    
    if args.build and not args.deploy:
        do_deploy = False
    elif args.deploy and not args.build:
        do_build = False
    
    # Print banner
    print_banner("SPOT ANALYZER SAM DEPLOYMENT")
    print_info(f"Stack: {STACK_NAME}")
    print_info(f"Region: {REGION}")
    print_info(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    overall_start = time.time()
    
    # Build Go binary first
    if do_build:
        print_banner("STEP 1: BUILD GO BINARY")
        if not build_go_binary():
            print_error("Go build failed. Aborting.")
            sys.exit(1)
        
        print_banner("STEP 2: SAM BUILD")
        if not run_command([SAM_CMD, "build"], "SAM Build"):
            print_error("SAM Build failed. Aborting.")
            sys.exit(1)
    
    # Deploy
    if do_deploy:
        step_num = "3" if do_build else "1"
        print_banner(f"STEP {step_num}: SAM DEPLOY")
        
        deploy_cmd = [
            SAM_CMD, "deploy",
            "--stack-name", STACK_NAME,
            "--region", REGION,
            "--capabilities", "CAPABILITY_IAM",
            "--no-confirm-changeset",
            "--no-fail-on-empty-changeset"
        ]
        
        if not run_command(deploy_cmd, "SAM Deploy"):
            print_error("Deploy failed.")
            sys.exit(1)
        
        # Save outputs after successful deploy
        save_stack_outputs()
    
    # Summary
    total_time = time.time() - overall_start
    print_banner("DEPLOYMENT COMPLETE")
    print_success(f"Total time: {total_time:.1f}s")
    
    if do_deploy:
        print_info(f"Dashboard: Check utils/lambda/{OUTPUT_FILE} for URLs")


if __name__ == "__main__":
    main()
