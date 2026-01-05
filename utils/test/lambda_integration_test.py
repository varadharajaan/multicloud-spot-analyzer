#!/usr/bin/env python3
"""
Spot Analyzer - Lambda Integration Test Suite
Author: Varadharajan
Tests Lambda deployment for all cloud providers (AWS, Azure, GCP)

Usage:
    python utils/test/lambda_integration_test.py                    # Auto-detect URL from stack-outputs.txt
    python utils/test/lambda_integration_test.py --url <url>        # Use specific Lambda URL
    python utils/test/lambda_integration_test.py --timeout 300      # Set request timeout (default: 120s)
"""

import subprocess
import json
import time
import sys
import os
import re
import argparse
from datetime import datetime
from typing import Dict, List, Tuple, Optional

# ANSI colors
class Colors:
    GREEN = '\033[92m'
    RED = '\033[91m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    CYAN = '\033[96m'
    BOLD = '\033[1m'
    END = '\033[0m'

# Project paths
PROJECT_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
STACK_OUTPUTS_PATH = os.path.join(PROJECT_ROOT, "utils", "lambda", "stack-outputs.txt")
LOGS_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "logs")


def print_banner():
    print(f"""
{Colors.CYAN}{Colors.BOLD}
   _____ ____   ___ _____      _    _   _    _    _  __   ____________ ____
  / ___/  _ \\ / _ \\_   _|    / \\  | \\ | |  / \\  | | \\ \\ / /__  / ____| __ \\
  \\___ \\ |_) | | | || |     / _ \\ |  \\| | / _ \\ | |  \\ V /  / /|  _| |  _) |
   ___) |  __/| |_| || |    / ___ \\| |\\  |/ ___ \\| |___| |  / /_| |___| | \\ \\
  |____/|_|    \\___/ |_|   /_/   \\_\\_| \\_/_/   \\_\\_____|_| /____|_____|_|  \\_\\

  +===========================================================================+
  |  [*] LAMBDA INTEGRATION TEST SUITE                                        |
  |      Author: Varadharajan | https://github.com/varadharajaan             |
  +===========================================================================+
{Colors.END}
""")


def get_lambda_url_from_outputs() -> Optional[str]:
    """Extract Lambda URL from stack-outputs.txt"""
    if not os.path.exists(STACK_OUTPUTS_PATH):
        print(f"{Colors.RED}[ERROR] Stack outputs file not found: {STACK_OUTPUTS_PATH}{Colors.END}")
        print(f"{Colors.YELLOW}[TIP] Run 'python utils/lambda/sam_deploy.py' first{Colors.END}")
        return None
    
    try:
        with open(STACK_OUTPUTS_PATH, 'r', encoding='utf-8') as f:
            content = f.read()
        
        # Look for Lambda URL pattern
        # Pattern: https://<id>.lambda-url.<region>.on.aws/
        match = re.search(r'https://[a-z0-9]+\.lambda-url\.[a-z0-9-]+\.on\.aws/?', content)
        if match:
            url = match.group(0).rstrip('/')
            return url
        
        print(f"{Colors.RED}[ERROR] Could not find Lambda URL in stack-outputs.txt{Colors.END}")
        return None
        
    except Exception as e:
        print(f"{Colors.RED}[ERROR] Failed to read stack outputs: {e}{Colors.END}")
        return None


def run_api_test(base_url: str, endpoint: str, body: dict, timeout: int = 120) -> Tuple[bool, dict, float]:
    """Run API test using PowerShell Invoke-RestMethod"""
    full_url = f"{base_url}{endpoint}"
    body_json = json.dumps(body).replace('"', '\\"')
    
    cmd = f'powershell -Command "$body = \'{body_json}\'; Invoke-RestMethod -Uri \'{full_url}\' -Method POST -Body $body -ContentType \'application/json\' -TimeoutSec {timeout} | ConvertTo-Json -Depth 10"'
    
    start = time.time()
    try:
        result = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            timeout=timeout + 30,  # Extra buffer for PowerShell overhead
            cwd=PROJECT_ROOT,
            encoding='utf-8',
            errors='replace'
        )
        duration = time.time() - start
        
        if result.returncode != 0:
            error_msg = result.stderr or result.stdout or "Unknown error"
            # Check for specific errors
            if "timeout" in error_msg.lower() or "timed out" in error_msg.lower():
                return False, {"error": f"Request timed out after {timeout}s"}, duration
            return False, {"error": error_msg[:500]}, duration
        
        try:
            data = json.loads(result.stdout)
            return True, data, duration
        except json.JSONDecodeError:
            return False, {"error": "Invalid JSON response", "raw": (result.stdout or "")[:500]}, duration
            
    except subprocess.TimeoutExpired:
        return False, {"error": f"Request timed out after {timeout}s"}, time.time() - start
    except Exception as e:
        return False, {"error": str(e)}, time.time() - start


class TestResult:
    def __init__(self, name: str, cloud: str, test_type: str):
        self.name = name
        self.cloud = cloud
        self.test_type = test_type
        self.passed = False
        self.duration = 0.0
        self.error = None
        self.response = None
    
    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "cloud": self.cloud,
            "type": self.test_type,
            "passed": self.passed,
            "duration_ms": round(self.duration * 1000, 2),
            "error": self.error
        }


class LambdaTestRunner:
    def __init__(self, base_url: str, timeout: int = 120):
        self.base_url = base_url.rstrip('/')
        self.timeout = timeout
        self.results: List[TestResult] = []
        self.start_time = datetime.now()
    
    def run_analyze_test(self, cloud: str, region: str, params: dict) -> TestResult:
        """Run analyze API test"""
        result = TestResult(f"{cloud.upper()} Analyze", cloud, "api")
        
        body = {
            "cloudProvider": cloud,
            "region": region,
            "os": "linux",
            **params
        }
        
        print(f"  {Colors.BLUE}Testing {cloud.upper()} Analyze API...{Colors.END}", end=" ", flush=True)
        
        success, response, duration = run_api_test(self.base_url, "/api/analyze", body, self.timeout)
        result.duration = duration
        result.response = response
        
        if success and response.get("success"):
            instances = response.get("instances", [])
            if instances and len(instances) > 0:
                result.passed = True
                print(f"{Colors.GREEN}✓ PASS{Colors.END} ({duration:.2f}s) - {len(instances)} instances")
            else:
                result.error = "No instances returned"
                print(f"{Colors.RED}✗ FAIL{Colors.END} - No instances returned")
        else:
            result.error = response.get("error", "Unknown error")
            print(f"{Colors.RED}✗ FAIL{Colors.END} - {result.error[:80]}")
        
        self.results.append(result)
        return result
    
    def run_az_test(self, cloud: str, region: str, instance_type: str) -> TestResult:
        """Run AZ recommendation API test"""
        result = TestResult(f"{cloud.upper()} AZ Lookup", cloud, "api")
        
        body = {
            "cloudProvider": cloud,
            "region": region,
            "instanceType": instance_type
        }
        
        print(f"  {Colors.BLUE}Testing {cloud.upper()} AZ Lookup API...{Colors.END}", end=" ", flush=True)
        
        # Use longer timeout for Azure (SKU fetch can be slow)
        az_timeout = self.timeout * 2 if cloud == "azure" else self.timeout
        
        success, response, duration = run_api_test(self.base_url, "/api/az", body, az_timeout)
        result.duration = duration
        result.response = response
        
        if success and response.get("success"):
            recommendations = response.get("recommendations", [])
            best_az = response.get("bestAz", "")
            if recommendations and len(recommendations) > 0 and best_az:
                result.passed = True
                print(f"{Colors.GREEN}✓ PASS{Colors.END} ({duration:.2f}s) - Best: {best_az}")
            elif response.get("usingRealData") == False and cloud in ["azure", "gcp"]:
                # Azure/GCP may not have real-time data
                result.passed = True
                print(f"{Colors.YELLOW}✓ PASS{Colors.END} ({duration:.2f}s) - Estimated data")
            else:
                result.error = "No AZ recommendations returned"
                print(f"{Colors.RED}✗ FAIL{Colors.END} - No recommendations")
        else:
            result.error = response.get("error", "Unknown error")
            print(f"{Colors.RED}✗ FAIL{Colors.END} - {result.error[:80]}")
        
        self.results.append(result)
        return result
    
    def run_all_tests(self):
        """Run all Lambda integration tests"""
        print(f"\n{Colors.BOLD}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}  Lambda URL: {self.base_url}{Colors.END}")
        print(f"{Colors.BOLD}  Timeout: {self.timeout}s{Colors.END}")
        print(f"{Colors.BOLD}{'='*60}{Colors.END}\n")
        
        # AWS Tests
        print(f"{Colors.CYAN}[AWS Tests]{Colors.END}")
        self.run_analyze_test("aws", "us-east-1", {"minVcpu": 2, "maxVcpu": 8, "minMemoryGb": 4, "maxMemoryGb": 32, "topN": 5})
        self.run_az_test("aws", "us-east-1", "m5.large")
        print()
        
        # Azure Tests
        print(f"{Colors.CYAN}[Azure Tests]{Colors.END}")
        self.run_analyze_test("azure", "eastus", {"minVcpu": 2, "maxVcpu": 8, "minMemoryGb": 4, "maxMemoryGb": 32, "topN": 5})
        self.run_az_test("azure", "eastus", "Standard_D4s_v3")
        print()
        
        # GCP Tests
        print(f"{Colors.CYAN}[GCP Tests]{Colors.END}")
        self.run_analyze_test("gcp", "us-central1", {"minVcpu": 2, "maxVcpu": 8, "minMemoryGb": 4, "maxMemoryGb": 32, "topN": 5})
        self.run_az_test("gcp", "us-central1", "n2-standard-4")
        print()
        
        # Summary
        passed = sum(1 for r in self.results if r.passed)
        failed = len(self.results) - passed
        total_duration = sum(r.duration for r in self.results)
        
        print(f"{Colors.BOLD}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}  SUMMARY{Colors.END}")
        print(f"{Colors.BOLD}{'='*60}{Colors.END}")
        print(f"  Total Tests: {len(self.results)}")
        print(f"  {Colors.GREEN}Passed: {passed}{Colors.END}")
        print(f"  {Colors.RED}Failed: {failed}{Colors.END}")
        print(f"  Duration: {total_duration:.2f}s")
        print(f"{Colors.BOLD}{'='*60}{Colors.END}\n")
        
        # Save report
        self.save_report(passed, failed, total_duration)
        
        return failed == 0
    
    def save_report(self, passed: int, failed: int, total_duration: float):
        """Save test report to logs folder"""
        os.makedirs(LOGS_DIR, exist_ok=True)
        
        timestamp = self.start_time.strftime('%Y-%m-%d_%H-%M-%S')
        
        # JSON report
        report = {
            "timestamp": self.start_time.isoformat(),
            "lambda_url": self.base_url,
            "summary": {
                "total": len(self.results),
                "passed": passed,
                "failed": failed,
                "duration_seconds": round(total_duration, 2)
            },
            "tests": [r.to_dict() for r in self.results]
        }
        
        json_path = os.path.join(LOGS_DIR, f"lambda-test-{timestamp}.json")
        with open(json_path, 'w', encoding='utf-8') as f:
            json.dump(report, f, indent=2)
        
        # Text report
        txt_path = os.path.join(LOGS_DIR, f"lambda-test-{timestamp}.txt")
        with open(txt_path, 'w', encoding='utf-8') as f:
            f.write(f"SPOT ANALYZER LAMBDA INTEGRATION TEST REPORT\n")
            f.write(f"{'='*60}\n")
            f.write(f"Timestamp: {self.start_time.isoformat()}\n")
            f.write(f"Lambda URL: {self.base_url}\n")
            f.write(f"Total: {len(self.results)} | Passed: {passed} | Failed: {failed}\n")
            f.write(f"Duration: {total_duration:.2f}s\n")
            f.write(f"{'='*60}\n\n")
            
            for r in self.results:
                status = "PASS" if r.passed else "FAIL"
                f.write(f"[{status}] {r.name} ({r.duration:.2f}s)\n")
                if r.error:
                    f.write(f"       Error: {r.error}\n")
        
        # Create latest symlinks
        for ext in ['json', 'txt']:
            latest = os.path.join(LOGS_DIR, f"lambda-test-latest.{ext}")
            src = os.path.join(LOGS_DIR, f"lambda-test-{timestamp}.{ext}")
            try:
                if os.path.exists(latest):
                    os.remove(latest)
                # On Windows, copy instead of symlink
                import shutil
                shutil.copy(src, latest)
            except:
                pass
        
        print(f"{Colors.GREEN}[OK] Report saved to {LOGS_DIR}{Colors.END}")


def main():
    parser = argparse.ArgumentParser(
        description="Lambda Integration Test Suite for Spot Analyzer",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    python utils/test/lambda_integration_test.py                    # Auto-detect URL
    python utils/test/lambda_integration_test.py --url <url>        # Use specific URL
    python utils/test/lambda_integration_test.py --timeout 300      # 5 min timeout
        """
    )
    parser.add_argument("--url", help="Lambda Function URL (auto-detected from stack-outputs.txt if not provided)")
    parser.add_argument("--timeout", type=int, default=120, help="Request timeout in seconds (default: 120)")
    
    args = parser.parse_args()
    
    print_banner()
    
    # Get Lambda URL
    if args.url:
        lambda_url = args.url.rstrip('/')
        print(f"{Colors.BLUE}[INFO] Using provided URL: {lambda_url}{Colors.END}")
    else:
        print(f"{Colors.BLUE}[INFO] Auto-detecting Lambda URL from stack-outputs.txt...{Colors.END}")
        lambda_url = get_lambda_url_from_outputs()
        if not lambda_url:
            sys.exit(1)
        print(f"{Colors.GREEN}[OK] Found URL: {lambda_url}{Colors.END}")
    
    # Run tests
    runner = LambdaTestRunner(lambda_url, args.timeout)
    success = runner.run_all_tests()
    
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
