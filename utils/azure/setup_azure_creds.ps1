<#
.SYNOPSIS
    Azure Credentials Setup Script for Spot Analyzer

.DESCRIPTION
    This script:
    1. Logs into Azure (if not already logged in)
    2. Creates a service principal (or reuses existing)
    3. Saves credentials to azure-config.yaml (separate from main config)
    4. Creates AWS Secrets Manager secret (optional)

.PARAMETER ServicePrincipalName
    Name of the service principal to create (default: spot-analyzer)

.PARAMETER ConfigPath
    Path to azure-config.yaml relative to script location

.PARAMETER AwsSecretName
    AWS Secrets Manager secret name

.PARAMETER SkipAwsSecret
    Skip AWS Secrets Manager creation

.EXAMPLE
    .\setup_azure_creds.ps1
    .\setup_azure_creds.ps1 -SkipAwsSecret
#>

param(
    [string]$ServicePrincipalName = "spot-analyzer",
    [string]$ConfigPath = "..\..\azure-config.yaml",
    [string]$AwsSecretName = "spot-analyzer/azure-credentials",
    [string]$AwsRegion = "us-east-1",
    [switch]$SkipAwsSecret
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
# Step 1: Check Azure CLI
# =============================================================================
Write-Status "Checking Azure CLI installation..."

$azCmd = Get-Command az -ErrorAction SilentlyContinue
if (-not $azCmd) {
    Write-Err "Azure CLI not found. Install it with: winget install Microsoft.AzureCLI"
    exit 1
}

$azVersionOutput = az version 2>$null | ConvertFrom-Json
Write-Success "Azure CLI found: $($azVersionOutput.'azure-cli')"

# =============================================================================
# Step 2: Login to Azure (if needed)
# =============================================================================
Write-Status "Checking Azure login status..."

$accountJson = az account show 2>$null
if (-not $accountJson) {
    Write-Status "Not logged in. Opening browser for Azure login..."
    az login | Out-Null
    $accountJson = az account show 2>$null
}

$account = $accountJson | ConvertFrom-Json
$subscriptionId = $account.id
$tenantId = $account.tenantId
$subscriptionName = $account.name

Write-Success "Logged in to Azure"
Write-Host "  Subscription: $subscriptionName"
Write-Host "  Subscription ID: $subscriptionId"
Write-Host "  Tenant ID: $tenantId"

# =============================================================================
# Step 3: Check if Service Principal exists
# =============================================================================
Write-Status "Checking for existing service principal: $ServicePrincipalName"

# Temporarily allow errors so az CLI warnings don't stop the script
$ErrorActionPreference = "Continue"

$existingSpJson = az ad sp list --display-name $ServicePrincipalName 2>&1 | Where-Object { $_ -notmatch "^WARNING:" }
$existingSp = $null
if ($existingSpJson) {
    try {
        $existingSp = $existingSpJson | ConvertFrom-Json
    } catch {
        $existingSp = $null
    }
}

if ($existingSp -and $existingSp.Count -gt 0) {
    $clientId = $existingSp[0].appId
    Write-Warn "Service principal already exists (Client ID: $clientId)"
    Write-Status "Creating new client secret for existing service principal..."
    
    # Create a new secret for existing SP
    $credOutput = az ad sp credential reset --id $clientId 2>&1
    $credJson = $credOutput | Where-Object { $_ -notmatch "^WARNING:" -and $_ -notmatch "^ERROR:" }
    if ($credJson) {
        try {
            $cred = $credJson | ConvertFrom-Json
            $clientId = $cred.appId
            $clientSecret = $cred.password
        } catch {
            Write-Err "Failed to parse credentials: $_"
            exit 1
        }
    } else {
        Write-Err "Failed to reset credentials"
        exit 1
    }
} else {
    Write-Status "Creating new service principal: $ServicePrincipalName"
    
    # Create new service principal with Reader role
    $spOutput = az ad sp create-for-rbac --name $ServicePrincipalName --role "Reader" --scopes "/subscriptions/$subscriptionId" 2>&1
    $spJson = $spOutput | Where-Object { $_ -notmatch "^WARNING:" -and $_ -notmatch "^ERROR:" }
    
    if (-not $spJson) {
        Write-Err "Failed to create service principal"
        Write-Host "Output: $spOutput"
        exit 1
    }
    
    try {
        $sp = $spJson | ConvertFrom-Json
        $clientId = $sp.appId
        $clientSecret = $sp.password
    } catch {
        Write-Err "Failed to parse service principal response: $_"
        exit 1
    }
    
    Write-Success "Service principal created"
}

# Restore error handling
$ErrorActionPreference = "Stop"

Write-Host "  Client ID: $clientId"
Write-Host "  Tenant ID: $tenantId"
Write-Host "  Subscription ID: $subscriptionId"

# =============================================================================
# Step 4: Create/Update azure-config.yaml
# =============================================================================
Write-Status "Saving Azure credentials to azure-config.yaml..."

$configFullPath = Join-Path $PSScriptRoot $ConfigPath
$configFullPath = [System.IO.Path]::GetFullPath($configFullPath)

# Create azure-config.yaml with credentials
$configContent = @"
# Azure credentials for Spot Analyzer
# This file contains secrets - DO NOT commit to source control
# Generated by setup_azure_creds.ps1

azure_credentials:
  tenant_id: "$tenantId"
  client_id: "$clientId"
  client_secret: "$clientSecret"
  subscription_id: "$subscriptionId"
"@

$configContent | Out-File -FilePath $configFullPath -Encoding utf8
Write-Success "Saved credentials to $configFullPath"

# =============================================================================
# Step 5: Create AWS Secrets Manager secret
# =============================================================================
if (-not $SkipAwsSecret) {
    Write-Status "Checking AWS CLI..."
    
    $awsCmd = Get-Command aws -ErrorAction SilentlyContinue
    if (-not $awsCmd) {
        Write-Warn "AWS CLI not found. Skipping Secrets Manager setup."
        Write-Host "  Run 'winget install Amazon.AWSCLI' to install AWS CLI."
        $SkipAwsSecret = $true
    } else {
        $awsIdentityJson = aws sts get-caller-identity 2>$null
        if (-not $awsIdentityJson) {
            Write-Warn "AWS CLI not configured. Skipping Secrets Manager setup."
            Write-Host "  Run 'aws configure' to set up AWS credentials."
            $SkipAwsSecret = $true
        } else {
            $awsIdentity = $awsIdentityJson | ConvertFrom-Json
            Write-Success "AWS CLI configured (Account: $($awsIdentity.Account))"
        }
    }
}

if (-not $SkipAwsSecret) {
    Write-Status "Creating/updating AWS Secrets Manager secret..."
    
    # Temporarily allow errors for AWS CLI
    $ErrorActionPreference = "Continue"
    
    $secretValue = @{
        AZURE_TENANT_ID = $tenantId
        AZURE_CLIENT_ID = $clientId
        AZURE_CLIENT_SECRET = $clientSecret
        AZURE_SUBSCRIPTION_ID = $subscriptionId
    } | ConvertTo-Json -Compress
    
    # Check if secret exists (suppress error output)
    $existingSecret = $null
    $checkResult = aws secretsmanager describe-secret --secret-id $AwsSecretName --region $AwsRegion 2>&1
    if ($LASTEXITCODE -eq 0) {
        $existingSecret = $checkResult
    }
    
    if ($existingSecret) {
        Write-Status "Updating existing secret..."
        $updateResult = aws secretsmanager put-secret-value --secret-id $AwsSecretName --secret-string $secretValue --region $AwsRegion 2>&1
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Updated secret: $AwsSecretName"
        } else {
            Write-Err "Failed to update secret: $updateResult"
        }
    } else {
        Write-Status "Creating new secret..."
        $createResult = aws secretsmanager create-secret --name $AwsSecretName --description "Azure credentials for Spot Analyzer" --secret-string $secretValue --region $AwsRegion 2>&1
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Created secret: $AwsSecretName"
        } else {
            Write-Err "Failed to create secret: $createResult"
        }
    }
    
    $ErrorActionPreference = "Stop"
    
    Write-Host ""
    Write-Host "  Secret ARN: arn:aws:secretsmanager:${AwsRegion}:$($awsIdentity.Account):secret:$AwsSecretName"
}

# =============================================================================
# Summary
# =============================================================================
Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Success "Azure credentials setup complete!"
Write-Host "============================================================" -ForegroundColor Green
Write-Host ""
Write-Host "Credentials saved to:"
Write-Host "  - azure-config.yaml: $configFullPath"
if (-not $SkipAwsSecret) {
    Write-Host "  - AWS Secrets Manager: $AwsSecretName"
}
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. Restart spot-analyzer to pick up new credentials"
Write-Host "  2. Azure SKU availability will now be used for AZ recommendations"
Write-Host ""

# Security reminder
Write-Warn "SECURITY REMINDER:"
Write-Host "  - azure-config.yaml is already in .gitignore"
Write-Host "  - Never commit credentials to source control"
Write-Host ""
