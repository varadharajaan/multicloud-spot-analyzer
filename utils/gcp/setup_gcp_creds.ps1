<#
.SYNOPSIS
    GCP Credentials Setup Script for Spot Analyzer

.DESCRIPTION
    This script:
    1. Checks for gcloud CLI installation
    2. Logs into GCP (if not already logged in)
    3. Creates a service account with required permissions
    4. Generates a JSON key for the service account
    5. Saves credentials to gcp-config.yaml
    6. Creates AWS Secrets Manager secret (optional, for Lambda deployment)
    
    REQUIRED GCP PERMISSIONS:
    The service account needs the following roles:
    - roles/compute.viewer       (Read VM/zone availability)
    - roles/billing.viewer       (Read Spot VM pricing - optional)

.PARAMETER ServiceAccountName
    Name of the service account to create (default: spot-analyzer)

.PARAMETER ConfigPath
    Path to gcp-config.yaml relative to script location

.PARAMETER AwsSecretName
    AWS Secrets Manager secret name for Lambda deployment

.PARAMETER SkipAwsSecret
    Skip AWS Secrets Manager creation

.PARAMETER ProjectId
    GCP Project ID (will prompt if not provided)

.PARAMETER ForceNewKey
    Force regeneration of service account key even if credentials exist

.EXAMPLE
    .\setup_gcp_creds.ps1
    .\setup_gcp_creds.ps1 -SkipAwsSecret
    .\setup_gcp_creds.ps1 -ProjectId my-project -ForceNewKey
#>

param(
    [string]$ServiceAccountName = "spot-analyzer",
    [string]$ConfigPath = "..\..\gcp-config.yaml",
    [string]$AwsSecretName = "spot-analyzer/gcp-credentials",
    [string]$AwsRegion = "us-east-1",
    [string]$ProjectId = "",
    [switch]$SkipAwsSecret,
    [switch]$ForceNewKey
)

$ErrorActionPreference = "Stop"

function Write-Status {
    param([string]$message)
    Write-Host "[*] $message" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$message)
    Write-Host "[OK] $message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$message)
    Write-Host "[!] $message" -ForegroundColor Yellow
}

function Write-Err {
    param([string]$message)
    Write-Host "[X] $message" -ForegroundColor Red
}

# =============================================================================
# Step 1: Check gcloud CLI
# =============================================================================
Write-Status "Checking gcloud CLI installation..."

$gcloudCmd = Get-Command gcloud -ErrorAction SilentlyContinue
if (-not $gcloudCmd) {
    Write-Err "gcloud CLI not found."
    Write-Host ""
    Write-Host "Install Google Cloud CLI:"
    Write-Host "  1. Download from: https://cloud.google.com/sdk/docs/install"
    Write-Host "  2. Or use winget: winget install Google.CloudSDK"
    Write-Host ""
    exit 1
}

$gcloudVersionOutput = gcloud version --format="value(Google Cloud SDK)" 2>$null
Write-Success "gcloud CLI found: $gcloudVersionOutput"

# =============================================================================
# Step 2: Login to GCP (if needed)
# =============================================================================
Write-Status "Checking GCP login status..."

$accountList = gcloud auth list --filter="status:ACTIVE" --format="value(account)" 2>$null
if (-not $accountList) {
    Write-Status "Not logged in. Opening browser for GCP login..."
    gcloud auth login --update-adc
    $accountList = gcloud auth list --filter="status:ACTIVE" --format="value(account)" 2>$null
}

if (-not $accountList) {
    Write-Err "Failed to authenticate with GCP"
    exit 1
}

Write-Success "Logged in as: $accountList"

# =============================================================================
# Step 3: Get or set Project ID
# =============================================================================
if (-not $ProjectId) {
    $ProjectId = gcloud config get-value project 2>$null
}

if (-not $ProjectId) {
    Write-Status "No project configured. Available projects:"
    gcloud projects list --format="table(projectId, name, projectNumber)"
    Write-Host ""
    $ProjectId = Read-Host "Enter Project ID"
    if (-not $ProjectId) {
        Write-Err "Project ID is required"
        exit 1
    }
}

# Set as current project
gcloud config set project $ProjectId 2>$null
Write-Success "Using project: $ProjectId"

# =============================================================================
# Step 4: Enable required APIs
# =============================================================================
Write-Status "Enabling required APIs..."

$apis = @(
    "compute.googleapis.com",
    "cloudbilling.googleapis.com"
)

foreach ($api in $apis) {
    $apiStatus = gcloud services list --filter="config.name:$api" --format="value(state)" 2>$null
    if ($apiStatus -eq "ENABLED") {
        Write-Success "  $api already enabled"
    } else {
        Write-Status "  Enabling $api..."
        gcloud services enable $api 2>$null
        if ($LASTEXITCODE -eq 0) {
            Write-Success "  $api enabled"
        } else {
            Write-Warn "  Could not enable $api (may require billing account)"
        }
    }
}

# =============================================================================
# Step 5: Check if Service Account exists
# =============================================================================
$serviceAccountEmail = "$ServiceAccountName@$ProjectId.iam.gserviceaccount.com"
Write-Status "Checking for service account: $serviceAccountEmail"

$configFullPath = Join-Path $PSScriptRoot $ConfigPath
$configFullPath = [System.IO.Path]::GetFullPath($configFullPath)

# Check if we already have credentials
$existingCreds = $null
if ((Test-Path $configFullPath) -and -not $ForceNewKey) {
    $configContent = Get-Content $configFullPath -Raw
    if ($configContent -match "service_account_json:") {
        Write-Success "Existing credentials found in gcp-config.yaml"
        Write-Host "  Use -ForceNewKey to regenerate"
        $existingCreds = $true
    }
}

$existingSa = gcloud iam service-accounts describe $serviceAccountEmail --format="value(email)" 2>$null

if ($existingSa) {
    Write-Success "Service account exists: $serviceAccountEmail"
} else {
    Write-Status "Creating service account: $ServiceAccountName"
    gcloud iam service-accounts create $ServiceAccountName `
        --display-name="Spot Analyzer" `
        --description="Service account for Spot Analyzer VM availability checks"
    
    if ($LASTEXITCODE -ne 0) {
        Write-Err "Failed to create service account"
        exit 1
    }
    Write-Success "Service account created"
}

# =============================================================================
# Step 6: Grant IAM roles
# =============================================================================
Write-Status "Granting IAM roles..."

$roles = @(
    "roles/compute.viewer",
    "roles/billing.viewer"
)

foreach ($role in $roles) {
    Write-Status "  Granting $role..."
    gcloud projects add-iam-policy-binding $ProjectId `
        --member="serviceAccount:$serviceAccountEmail" `
        --role="$role" `
        --quiet 2>$null | Out-Null
    
    if ($LASTEXITCODE -eq 0) {
        Write-Success "  $role granted"
    } else {
        Write-Warn "  Could not grant $role (may need additional permissions)"
    }
}

# =============================================================================
# Step 7: Generate service account key (if needed)
# =============================================================================
$keyJson = $null

if ($existingCreds -and -not $ForceNewKey) {
    Write-Success "Using existing credentials from gcp-config.yaml"
} else {
    Write-Status "Generating new service account key..."
    
    $keyFile = Join-Path $env:TEMP "gcp-sa-key-$(Get-Random).json"
    gcloud iam service-accounts keys create $keyFile `
        --iam-account=$serviceAccountEmail 2>$null
    
    if ($LASTEXITCODE -ne 0) {
        Write-Err "Failed to create service account key"
        exit 1
    }
    
    $keyJson = Get-Content $keyFile -Raw
    Remove-Item $keyFile -Force -ErrorAction SilentlyContinue
    
    Write-Success "Service account key generated"
    
    # =============================================================================
    # Step 8: Save to gcp-config.yaml
    # =============================================================================
    Write-Status "Saving credentials to gcp-config.yaml..."
    
    # Escape JSON for YAML (convert to single line and escape quotes)
    $keyJsonEscaped = $keyJson -replace "`r`n", "" -replace "`n", "" -replace '"', '\"'
    $keyJsonOneLine = ($keyJson | ConvertFrom-Json | ConvertTo-Json -Compress)
    
    $configContent = @"
# GCP Configuration for Spot Analyzer
# Generated by setup_gcp_creds.ps1 on $(Get-Date -Format "yyyy-MM-dd HH:mm:ss")
# WARNING: This file contains sensitive credentials - DO NOT commit to source control

gcp:
  # GCP Project ID
  project_id: "$ProjectId"
  
  # Service Account Email
  service_account_email: "$serviceAccountEmail"
  
  # Service Account JSON Key (inline, single line)
  # This is used by the spot-analyzer for authenticated API access
  service_account_json: '$keyJsonOneLine'
  
  # Enabled features
  features:
    # Use Compute Engine API for real zone availability
    compute_api: true
    # Use Cloud Billing API for real-time Spot pricing
    billing_api: true

# Data sources configuration
data_sources:
  # Try real API first, fall back to estimates if unavailable
  zone_availability: "api"  # "api" or "estimated"
  pricing: "api"            # "api" or "estimated"
"@
    
    # Ensure directory exists
    $configDir = Split-Path $configFullPath -Parent
    if (-not (Test-Path $configDir)) {
        New-Item -ItemType Directory -Path $configDir -Force | Out-Null
    }
    
    [System.IO.File]::WriteAllText($configFullPath, $configContent)
    Write-Success "Credentials saved to: $configFullPath"
}

# =============================================================================
# Step 9: Create AWS Secrets Manager secret (optional)
# =============================================================================
if (-not $SkipAwsSecret) {
    Write-Host ""
    Write-Status "Setting up AWS Secrets Manager..."
    
    # Check for AWS CLI
    $awsCmd = Get-Command aws -ErrorAction SilentlyContinue
    if (-not $awsCmd) {
        Write-Warn "AWS CLI not found - skipping Secrets Manager setup"
        Write-Host "  Install AWS CLI: winget install Amazon.AWSCLI"
    } else {
        # Check AWS credentials
        $awsIdentity = $null
        try {
            $awsIdentityJson = aws sts get-caller-identity 2>$null
            if ($LASTEXITCODE -eq 0) {
                $awsIdentity = $awsIdentityJson | ConvertFrom-Json
            }
        } catch { }
        
        if (-not $awsIdentity) {
            Write-Warn "AWS credentials not configured - skipping Secrets Manager"
            Write-Host "  Run: aws configure"
        } else {
            Write-Success "AWS account: $($awsIdentity.Account)"
            
            # Prepare secret value - either use new key or load existing
            if ($keyJson) {
                $gcpKeyJson = $keyJson
            } else {
                # Load from existing config
                if (Test-Path $configFullPath) {
                    $configContent = Get-Content $configFullPath -Raw
                    if ($configContent -match "service_account_json:\s*'(.+?)'") {
                        $gcpKeyJson = $Matches[1]
                    }
                }
            }
            
            if (-not $gcpKeyJson) {
                Write-Warn "No GCP credentials to store in Secrets Manager"
            } else {
                $ErrorActionPreference = "Continue"
                
                # Create secret JSON structure
                $secretValue = @{
                    GOOGLE_APPLICATION_CREDENTIALS_JSON = $gcpKeyJson
                    GCP_PROJECT_ID = $ProjectId
                } | ConvertTo-Json -Compress
                
                $tempFile = Join-Path $env:TEMP "gcp-secret-$(Get-Random).json"
                [System.IO.File]::WriteAllText($tempFile, $secretValue)
                
                # Check if secret exists
                $existingSecret = $null
                $checkResult = aws secretsmanager get-secret-value --secret-id $AwsSecretName --region $AwsRegion 2>&1
                if ($LASTEXITCODE -eq 0) {
                    try {
                        $existingSecret = $checkResult | ConvertFrom-Json
                    } catch { }
                }
                
                if ($existingSecret) {
                    # Update existing secret
                    Write-Status "Updating existing secret: $AwsSecretName"
                    $updateResult = aws secretsmanager put-secret-value `
                        --secret-id $AwsSecretName `
                        --secret-string "file://$tempFile" `
                        --region $AwsRegion 2>&1
                    
                    if ($LASTEXITCODE -eq 0) {
                        Write-Success "Updated secret: $AwsSecretName"
                    } else {
                        Write-Err "Failed to update secret: $updateResult"
                    }
                } else {
                    # Create new secret
                    Write-Status "Creating new secret: $AwsSecretName"
                    $createResult = aws secretsmanager create-secret `
                        --name $AwsSecretName `
                        --description "GCP credentials for Spot Analyzer" `
                        --secret-string "file://$tempFile" `
                        --region $AwsRegion 2>&1
                    
                    if ($LASTEXITCODE -eq 0) {
                        Write-Success "Created secret: $AwsSecretName"
                    } else {
                        Write-Err "Failed to create secret: $createResult"
                    }
                }
                
                # Cleanup temp file
                Remove-Item $tempFile -Force -ErrorAction SilentlyContinue
                
                $ErrorActionPreference = "Stop"
                
                Write-Host ""
                Write-Host "  Secret ARN: arn:aws:secretsmanager:${AwsRegion}:$($awsIdentity.Account):secret:$AwsSecretName"
            }
        }
    }
}

# =============================================================================
# Summary
# =============================================================================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Success "GCP credentials setup complete!"
Write-Host "============================================================" -ForegroundColor Green
Write-Host ""
Write-Host "Configuration:"
Write-Host "  Project ID: $ProjectId"
Write-Host "  Service Account: $serviceAccountEmail"
Write-Host ""
Write-Host "Credentials saved to:"
Write-Host "  - gcp-config.yaml: $configFullPath"
if (-not $SkipAwsSecret) {
    Write-Host "  - AWS Secrets Manager: $AwsSecretName"
}
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. Restart spot-analyzer to pick up new credentials"
Write-Host "  2. GCP zone availability will now use real Compute Engine API"
Write-Host "  3. GCP pricing will now use real Cloud Billing Catalog API"
Write-Host ""
Write-Host "For Lambda deployment:"
Write-Host "  1. Ensure AWS Secrets Manager has both Azure and GCP credentials"
Write-Host "  2. Run: python utils\lambda\sam_deploy.py"
Write-Host "  3. Lambda will automatically load credentials from Secrets Manager"
Write-Host ""
Write-Host "GCP permissions granted:"
Write-Host "  - roles/compute.viewer (VM/zone availability)"
Write-Host "  - roles/billing.viewer (Spot VM pricing)"
Write-Host ""

# Security reminder
Write-Warn "SECURITY REMINDER:"
Write-Host "  - gcp-config.yaml is already in .gitignore"
Write-Host "  - Never commit credentials to source control"
Write-Host "  - Rotate service account keys periodically"
Write-Host ""
