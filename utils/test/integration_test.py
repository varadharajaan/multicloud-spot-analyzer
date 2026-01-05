#!/usr/bin/env python3
"""
Spot Analyzer - Integration Test Suite
Author: Varadharajan
Runs integration tests for all cloud providers (AWS, Azure, GCP)
"""

import subprocess
import json
import time
import sys
import os
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

def print_banner():
    print(f"""
{Colors.CYAN}{Colors.BOLD}
   _____ ____   ___ _____      _    _   _    _    _  __   ____________ ____
  / ___/  _ \\ / _ \\_   _|    / \\  | \\ | |  / \\  | | \\ \\ / /__  / ____| __ \\
  \\___ \\ |_) | | | || |     / _ \\ |  \\| | / _ \\ | |  \\ V /  / /|  _| |  _) |
   ___) |  __/| |_| || |    / ___ \\| |\\  |/ ___ \\| |___| |  / /_| |___| | \\ \\
  |____/|_|    \\___/ |_|   /_/   \\_\\_| \\_/_/   \\_\\_____|_| /____|_____|_|  \\_\\

  +===========================================================================+
  |  [*] INTEGRATION TEST SUITE                                               |
  |      Author: Varadharajan | https://github.com/varadharajaan             |
  +===========================================================================+
{Colors.END}
""")

def run_command(cmd: str, timeout: int = 120) -> Tuple[bool, str, float]:
    """Run a command and return (success, output, duration)"""
    start = time.time()
    try:
        result = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            timeout=timeout,
            cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            encoding='utf-8',
            errors='replace'  # Replace undecodable characters
        )
        duration = time.time() - start
        output = (result.stdout or '') + (result.stderr or '')
        return result.returncode == 0, output, duration
    except subprocess.TimeoutExpired:
        return False, f"Command timed out after {timeout}s", time.time() - start
    except Exception as e:
        return False, str(e), time.time() - start

def run_powershell_api_test(endpoint: str, body: dict, timeout: int = 60) -> Tuple[bool, dict, float]:
    """Run API test using PowerShell Invoke-RestMethod"""
    body_json = json.dumps(body).replace('"', '\\"')
    cmd = f'powershell -Command "$body = \'{body_json}\'; Invoke-RestMethod -Uri \'http://localhost:8000{endpoint}\' -Method POST -Body $body -ContentType \'application/json\' -TimeoutSec {timeout} | ConvertTo-Json -Depth 10"'
    
    start = time.time()
    try:
        result = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            timeout=timeout + 10,
            cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            encoding='utf-8',
            errors='replace'
        )
        duration = time.time() - start
        
        if result.returncode != 0:
            return False, {"error": result.stderr or "Unknown error"}, duration
        
        try:
            data = json.loads(result.stdout)
            return True, data, duration
        except json.JSONDecodeError:
            return False, {"error": "Invalid JSON response", "raw": (result.stdout or "")[:500]}, duration
            
    except subprocess.TimeoutExpired:
        return False, {"error": f"Request timed out after {timeout}s"}, time.time() - start
    except Exception as e:
        return False, {"error": str(e)}, time.time() - start

def run_cli_test(args: str, timeout: int = 60) -> Tuple[bool, str, float]:
    """Run CLI test"""
    cmd = f".\\spot-cli.exe {args}"
    return run_command(cmd, timeout)

class TestResult:
    def __init__(self, name: str, category: str, cloud: str):
        self.name = name
        self.category = category
        self.cloud = cloud
        self.passed = False
        self.duration = 0.0
        self.error = None
        self.details = {}

    def __str__(self):
        status = f"{Colors.GREEN}✅ PASS{Colors.END}" if self.passed else f"{Colors.RED}❌ FAIL{Colors.END}"
        return f"{status} [{self.cloud:6}] {self.name} ({self.duration:.2f}s)"

    def to_dict(self):
        return {
            "name": self.name,
            "category": self.category,
            "cloud": self.cloud,
            "passed": self.passed,
            "duration": round(self.duration, 2),
            "error": self.error,
            "details": self.details
        }

class IntegrationTestSuite:
    def __init__(self):
        self.results: List[TestResult] = []
        self.start_time = None
        self.server_started = False

    def add_result(self, result: TestResult):
        self.results.append(result)
        print(f"  {result}")
        if not result.passed and result.error:
            print(f"    {Colors.YELLOW}Error: {result.error[:200]}{Colors.END}")

    def build_project(self) -> bool:
        """Build the Go project"""
        print(f"\n{Colors.BLUE}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}📦 STEP 1: Building Project{Colors.END}")
        print(f"{Colors.BLUE}{'='*60}{Colors.END}\n")

        # Build web server
        print("  Building web server...")
        success, output, duration = run_command("go build -o spot-analyzer.exe .\\cmd\\web\\main.go")
        if not success:
            print(f"  {Colors.RED}❌ Web server build failed{Colors.END}")
            print(f"  {output[:500]}")
            return False
        print(f"  {Colors.GREEN}✅ Web server built ({duration:.2f}s){Colors.END}")

        # Build CLI
        print("  Building CLI...")
        success, output, duration = run_command("go build -o spot-cli.exe .\\main.go")
        if not success:
            print(f"  {Colors.RED}❌ CLI build failed{Colors.END}")
            print(f"  {output[:500]}")
            return False
        print(f"  {Colors.GREEN}✅ CLI built ({duration:.2f}s){Colors.END}")

        return True

    def check_server(self) -> bool:
        """Check if server is running"""
        success, _, _ = run_powershell_api_test("/api/cache/status", {}, timeout=5)
        # GET request won't work with POST, try a simple check
        cmd = 'powershell -Command "try { Invoke-WebRequest -Uri \'http://localhost:8000/api/cache/status\' -TimeoutSec 5 | Out-Null; Write-Output \'OK\' } catch { Write-Output \'FAIL\' }"'
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
        return "OK" in result.stdout

    def start_server(self) -> bool:
        """Start the web server if not running"""
        print(f"\n{Colors.BLUE}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}🚀 STEP 2: Starting Server{Colors.END}")
        print(f"{Colors.BLUE}{'='*60}{Colors.END}\n")

        if self.check_server():
            print(f"  {Colors.GREEN}✅ Server already running on port 8000{Colors.END}")
            self.server_started = True
            return True

        print("  Starting server...")
        # Start server in background
        subprocess.Popen(
            ".\\spot-analyzer.exe",
            shell=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        )

        # Wait for server to start
        for i in range(10):
            time.sleep(1)
            if self.check_server():
                print(f"  {Colors.GREEN}✅ Server started on port 8000{Colors.END}")
                self.server_started = True
                return True
            print(f"  Waiting for server... ({i+1}/10)")

        print(f"  {Colors.RED}❌ Failed to start server{Colors.END}")
        return False

    def run_api_tests(self):
        """Run API endpoint tests"""
        print(f"\n{Colors.BLUE}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}🌐 STEP 3: API Endpoint Tests{Colors.END}")
        print(f"{Colors.BLUE}{'='*60}{Colors.END}\n")

        # Test cases: (name, endpoint, body, validation_fn)
        api_tests = [
            # Analyze API
            ("Analyze API", "/api/analyze", 
             {"cloudProvider": "aws", "minVcpu": 2, "maxVcpu": 8, "minMemory": 4, "region": "us-east-1", "topN": 3},
             lambda d: d.get("success") == True and len(d.get("instances", [])) > 0, "aws"),
            
            ("Analyze API", "/api/analyze",
             {"cloudProvider": "azure", "minVcpu": 2, "maxVcpu": 16, "minMemory": 4, "region": "eastus", "topN": 3, "maxInterruption": 4},
             lambda d: d.get("success") == True, "azure"),
            
            ("Analyze API", "/api/analyze",
             {"cloudProvider": "gcp", "minVcpu": 2, "maxVcpu": 8, "minMemory": 4, "region": "us-central1", "topN": 3},
             lambda d: d.get("success") == True and len(d.get("instances", [])) > 0, "gcp"),
            
            # AZ Lookup API
            ("AZ Lookup API", "/api/az",
             {"cloudProvider": "aws", "instanceType": "m5.large", "region": "us-east-1"},
             lambda d: d.get("success") == True and len(d.get("recommendations", [])) > 0, "aws"),
            
            ("AZ Lookup API", "/api/az",
             {"cloudProvider": "azure", "instanceType": "Standard_D2s_v5", "region": "eastus"},
             lambda d: d.get("success") == True and len(d.get("recommendations", [])) > 0, "azure"),
            
            ("AZ Lookup API", "/api/az",
             {"cloudProvider": "gcp", "instanceType": "n2-standard-4", "region": "us-central1"},
             lambda d: d.get("success") == True and len(d.get("recommendations", [])) > 0, "gcp"),
        ]

        for name, endpoint, body, validator, cloud in api_tests:
            result = TestResult(name, "API", cloud)
            success, data, duration = run_powershell_api_test(endpoint, body, timeout=90)
            result.duration = duration
            
            if success and validator(data):
                result.passed = True
                if "instances" in data:
                    result.details["count"] = len(data["instances"])
                if "recommendations" in data:
                    result.details["count"] = len(data["recommendations"])
            else:
                result.passed = False
                result.error = data.get("error", str(data)[:200]) if isinstance(data, dict) else str(data)[:200]
            
            self.add_result(result)

    def run_cli_tests(self):
        """Run CLI tests"""
        print(f"\n{Colors.BLUE}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}💻 STEP 4: CLI Tests{Colors.END}")
        print(f"{Colors.BLUE}{'='*60}{Colors.END}\n")

        cli_tests = [
            # Analyze CLI
            ("CLI Analyze", "analyze --cloud aws --region us-east-1 --vcpu 2 --memory 4 --top 3",
             lambda o: "TOP RECOMMENDATION" in o and "i4i" in o.lower() or "m5" in o.lower() or "instances" in o.lower(), "aws"),
            
            ("CLI Analyze", "analyze --cloud azure --region eastus --vcpu 2 --memory 4 --top 3 --max-interruption 4",
             lambda o: "TOP RECOMMENDATION" in o or "Found" in o, "azure"),
            
            ("CLI Analyze", "analyze --cloud gcp --region us-central1 --vcpu 2 --memory 4 --top 3",
             lambda o: "TOP RECOMMENDATION" in o and ("t2a" in o.lower() or "n2" in o.lower() or "e2" in o.lower()), "gcp"),
            
            # AZ CLI
            ("CLI AZ Lookup", "az --cloud aws --instance m5.large --region us-east-1",
             lambda o: "AVAILABILITY ZONE RECOMMENDATIONS" in o and "us-east-1" in o, "aws"),
            
            ("CLI AZ Lookup", "az --cloud azure --instance Standard_D2s_v5 --region eastus",
             lambda o: "AVAILABILITY ZONE RECOMMENDATIONS" in o and "eastus" in o, "azure"),
            
            ("CLI AZ Lookup", "az --cloud gcp --instance n2-standard-4 --region us-central1",
             lambda o: "AVAILABILITY ZONE RECOMMENDATIONS" in o and "us-central1" in o, "gcp"),
        ]

        for name, args, validator, cloud in cli_tests:
            result = TestResult(name, "CLI", cloud)
            success, output, duration = run_cli_test(args, timeout=90)
            result.duration = duration
            
            if success and validator(output):
                result.passed = True
            else:
                result.passed = False
                result.error = output[:300] if not success else "Validation failed"
            
            self.add_result(result)

    def print_summary(self):
        """Print test summary"""
        total = len(self.results)
        passed = sum(1 for r in self.results if r.passed)
        failed = total - passed
        total_duration = sum(r.duration for r in self.results)

        print(f"\n{Colors.BLUE}{'='*60}{Colors.END}")
        print(f"{Colors.BOLD}📊 TEST SUMMARY{Colors.END}")
        print(f"{Colors.BLUE}{'='*60}{Colors.END}\n")

        # By category
        categories = {}
        for r in self.results:
            key = f"{r.category}"
            if key not in categories:
                categories[key] = {"passed": 0, "failed": 0}
            if r.passed:
                categories[key]["passed"] += 1
            else:
                categories[key]["failed"] += 1

        print(f"  {'Category':<20} {'Passed':<10} {'Failed':<10}")
        print(f"  {'-'*40}")
        for cat, counts in categories.items():
            p_color = Colors.GREEN if counts["passed"] > 0 else ""
            f_color = Colors.RED if counts["failed"] > 0 else ""
            print(f"  {cat:<20} {p_color}{counts['passed']:<10}{Colors.END} {f_color}{counts['failed']:<10}{Colors.END}")

        # By cloud
        print(f"\n  {'Cloud':<20} {'Passed':<10} {'Failed':<10}")
        print(f"  {'-'*40}")
        clouds = {}
        for r in self.results:
            if r.cloud not in clouds:
                clouds[r.cloud] = {"passed": 0, "failed": 0}
            if r.passed:
                clouds[r.cloud]["passed"] += 1
            else:
                clouds[r.cloud]["failed"] += 1
        
        for cloud, counts in clouds.items():
            p_color = Colors.GREEN if counts["passed"] > 0 else ""
            f_color = Colors.RED if counts["failed"] > 0 else ""
            print(f"  {cloud.upper():<20} {p_color}{counts['passed']:<10}{Colors.END} {f_color}{counts['failed']:<10}{Colors.END}")

        # Overall
        print(f"\n  {Colors.BOLD}{'='*40}{Colors.END}")
        status_color = Colors.GREEN if failed == 0 else Colors.YELLOW if passed > failed else Colors.RED
        print(f"  {Colors.BOLD}TOTAL:{Colors.END} {status_color}{passed}/{total} passed{Colors.END} ({total_duration:.2f}s)")
        
        if failed > 0:
            print(f"\n  {Colors.RED}Failed Tests:{Colors.END}")
            for r in self.results:
                if not r.passed:
                    print(f"    - [{r.cloud}] {r.name}")

        # Final status
        print(f"\n{Colors.BLUE}{'='*60}{Colors.END}")
        if failed == 0:
            print(f"{Colors.GREEN}{Colors.BOLD}🎉 ALL TESTS PASSED!{Colors.END}")
        else:
            print(f"{Colors.YELLOW}{Colors.BOLD}⚠️  {failed} TEST(S) FAILED{Colors.END}")
        print(f"{Colors.BLUE}{'='*60}{Colors.END}\n")

        # Save report
        self.save_report(passed, failed, total_duration)

        return failed == 0

    def save_report(self, passed: int, failed: int, total_duration: float):
        """Save test report to JSON and text files"""
        report_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "logs")
        os.makedirs(report_dir, exist_ok=True)
        
        timestamp = self.start_time.strftime('%Y-%m-%d_%H-%M-%S')
        
        # JSON report
        json_report = {
            "timestamp": self.start_time.isoformat(),
            "summary": {
                "total": len(self.results),
                "passed": passed,
                "failed": failed,
                "duration_seconds": round(total_duration, 2),
                "success": failed == 0
            },
            "by_category": {},
            "by_cloud": {},
            "tests": [r.to_dict() for r in self.results]
        }
        
        # Aggregate by category
        for r in self.results:
            if r.category not in json_report["by_category"]:
                json_report["by_category"][r.category] = {"passed": 0, "failed": 0}
            if r.passed:
                json_report["by_category"][r.category]["passed"] += 1
            else:
                json_report["by_category"][r.category]["failed"] += 1
        
        # Aggregate by cloud
        for r in self.results:
            if r.cloud not in json_report["by_cloud"]:
                json_report["by_cloud"][r.cloud] = {"passed": 0, "failed": 0}
            if r.passed:
                json_report["by_cloud"][r.cloud]["passed"] += 1
            else:
                json_report["by_cloud"][r.cloud]["failed"] += 1
        
        json_path = os.path.join(report_dir, f"integration-test-{timestamp}.json")
        with open(json_path, 'w', encoding='utf-8') as f:
            json.dump(json_report, f, indent=2)
        
        # Text report
        txt_path = os.path.join(report_dir, f"integration-test-{timestamp}.txt")
        with open(txt_path, 'w', encoding='utf-8') as f:
            f.write("=" * 60 + "\n")
            f.write("SPOT ANALYZER - INTEGRATION TEST REPORT\n")
            f.write("=" * 60 + "\n\n")
            f.write(f"Timestamp: {self.start_time.strftime('%Y-%m-%d %H:%M:%S')}\n")
            f.write(f"Duration: {total_duration:.2f}s\n")
            f.write(f"Result: {'PASSED' if failed == 0 else 'FAILED'}\n\n")
            f.write(f"Summary: {passed}/{len(self.results)} tests passed\n\n")
            
            f.write("-" * 40 + "\n")
            f.write("TEST RESULTS\n")
            f.write("-" * 40 + "\n")
            for r in self.results:
                status = "PASS" if r.passed else "FAIL"
                f.write(f"[{status}] [{r.cloud.upper():6}] {r.name} ({r.duration:.2f}s)\n")
                if r.error:
                    f.write(f"       Error: {r.error[:100]}\n")
            
            if failed > 0:
                f.write("\n" + "-" * 40 + "\n")
                f.write("FAILED TESTS\n")
                f.write("-" * 40 + "\n")
                for r in self.results:
                    if not r.passed:
                        f.write(f"- [{r.cloud}] {r.name}\n")
        
        # Also save as latest
        latest_json = os.path.join(report_dir, "integration-test-latest.json")
        latest_txt = os.path.join(report_dir, "integration-test-latest.txt")
        with open(latest_json, 'w', encoding='utf-8') as f:
            json.dump(json_report, f, indent=2)
        with open(latest_txt, 'w', encoding='utf-8') as f:
            with open(txt_path, 'r', encoding='utf-8') as src:
                f.write(src.read())
        
        print(f"  📄 Report saved to:")
        print(f"     - {json_path}")
        print(f"     - {txt_path}")

    def run(self) -> bool:
        """Run the full test suite"""
        self.start_time = datetime.now()
        print_banner()
        print(f"  Started at: {self.start_time.strftime('%Y-%m-%d %H:%M:%S')}")

        # Step 1: Build
        if not self.build_project():
            print(f"\n{Colors.RED}Build failed, aborting tests{Colors.END}")
            return False

        # Step 2: Start server
        if not self.start_server():
            print(f"\n{Colors.RED}Server failed to start, aborting API tests{Colors.END}")
            # Still run CLI tests
            self.run_cli_tests()
            self.print_summary()
            return False

        # Step 3: API tests
        self.run_api_tests()

        # Step 4: CLI tests
        self.run_cli_tests()

        # Summary
        return self.print_summary()


if __name__ == "__main__":
    suite = IntegrationTestSuite()
    success = suite.run()
    sys.exit(0 if success else 1)
