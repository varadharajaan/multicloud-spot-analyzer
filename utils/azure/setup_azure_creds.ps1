<#
.SYNOPSIS
    Azure Credentials Setup Script for Spot Analyzer

.DESCRIPTION
    This script:
    1. Logs into Azure (if not already logged in)
    2. Creates a service principal with required permissions (or reuses existing)
    3. Saves credentials to azure-config.yaml (separate from main config)
    4. Creates AWS Secrets Manager secret (optional, for Lambda deployment)
    
    If credentials already exist locally, they will be reused.
    Use -ForceNewSecret to regenerate the client secret.

    REQUIRED AZURE PERMISSIONS:
    The service principal needs the following permissions:
    - Microsoft.Compute/skus/read          (Read VM SKU availability)
    - Microsoft.Compute/locations/*/read   (Read region info)
    - Microsoft.Resources/subscriptions/read (Read subscription info)
    
    These are included in the "Reader" role which is assigned by default.
    For minimal permissions, you can create a custom role.

.PARAMETER ServicePrincipalName
    Name of the service principal to create (default: spot-analyzer)

.PARAMETER RequiredAccount
    Required Azure account email. If specified, will logout and login with this account.
    If current account is @microsoft.com and no RequiredAccount specified, script will error.

.PARAMETER ConfigPath
    Path to azure-config.yaml relative to script location

.PARAMETER AwsSecretName
    AWS Secrets Manager secret name for Lambda deployment

.PARAMETER SkipAwsSecret
    Skip AWS Secrets Manager creation

.PARAMETER ForceNewSecret
    Force regeneration of client secret even if credentials exist

.EXAMPLE
    .\setup_azure_creds.ps1
    .\setup_azure_creds.ps1 -SkipAwsSecret
    .\setup_azure_creds.ps1 -ForceNewSecret
    .\setup_azure_creds.ps1 -RequiredAccount "user@outlook.com" -SkipAwsSecret
#>

param(
    [string]$ServicePrincipalName = "spot-analyzer",
    [string]$ConfigPath = "..\..\azure-config.yaml",
    [string]$AwsSecretName = "spot-analyzer/azure-credentials",
    [string]$AwsRegion = "us-east-1",
    [string]$RequiredAccount = "",  # Required Azure account email (e.g., "user@outlook.com")
    [switch]$SkipAwsSecret,
    [switch]$ForceNewSecret
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
# Step 2: Login to Azure (with account validation)
# =============================================================================
Write-Status "Checking Azure login status..."

$needsLogin = $false
$accountJson = az account show 2>$null

if ($accountJson) {
    $account = $accountJson | ConvertFrom-Json
    $currentUser = $account.user.name
    
    # Check if we need a specific account
    if ($RequiredAccount -ne "") {
        if ($currentUser -ne $RequiredAccount) {
            Write-Warn "Currently logged in as: $currentUser"
            Write-Warn "Required account: $RequiredAccount"
            Write-Status "Logging out and switching accounts..."
            az logout 2>$null
            $needsLogin = $true
        } else {
            Write-Success "Logged in as required account: $currentUser"
        }
    } else {
        # Check for Microsoft corporate account (block by default for personal projects)
        if ($currentUser -like "*@microsoft.com") {
            Write-Err "Currently logged in with Microsoft corporate account: $currentUser"
            Write-Err "For personal Azure subscriptions, please specify -RequiredAccount parameter"
            Write-Host ""
            Write-Host "Example:" -ForegroundColor Yellow
            Write-Host "  .\setup_azure_creds.ps1 -RequiredAccount 'your-email@outlook.com' -SkipAwsSecret" -ForegroundColor Yellow
            Write-Host ""
            exit 1
        }
        Write-Success "Logged in as: $currentUser"
    }
} else {
    $needsLogin = $true
}

if ($needsLogin) {
    if ($RequiredAccount -ne "") {
        Write-Status "Opening browser for Azure login as: $RequiredAccount"
        Write-Host "  Please select: $RequiredAccount in the browser" -ForegroundColor Yellow
    } else {
        Write-Status "Opening browser for Azure login..."
    }
    az login | Out-Null
    $accountJson = az account show 2>$null
    
    if (-not $accountJson) {
        Write-Err "Failed to login to Azure"
        exit 1
    }
    
    $account = $accountJson | ConvertFrom-Json
    
    # Verify correct account after login
    if ($RequiredAccount -ne "" -and $account.user.name -ne $RequiredAccount) {
        Write-Err "Logged in as wrong account: $($account.user.name)"
        Write-Err "Expected: $RequiredAccount"
        Write-Host ""
        Write-Host "Please run 'az logout' and try again, selecting the correct account." -ForegroundColor Yellow
        exit 1
    }
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

# Check if we already have credentials in azure-config.yaml
$configFullPath = Join-Path $PSScriptRoot $ConfigPath
$configFullPath = [System.IO.Path]::GetFullPath($configFullPath)
$existingCreds = $null

if (Test-Path $configFullPath) {
    $configContent = Get-Content $configFullPath -Raw
    if ($configContent -match "client_secret:\s*`"(.+?)`"") {
        $existingClientSecret = $Matches[1]
    }
    if ($configContent -match "client_id:\s*`"(.+?)`"") {
        $existingClientId = $Matches[1]
    }
    if ($existingClientSecret -and $existingClientId) {
        $existingCreds = @{
            ClientId = $existingClientId
            ClientSecret = $existingClientSecret
        }
    }
}

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
    
    # Check if we have existing credentials that match this SP
    if ($existingCreds -and $existingCreds.ClientId -eq $clientId -and -not $ForceNewSecret) {
        Write-Success "Service principal exists and credentials found in azure-config.yaml"
        Write-Host "  Using existing credentials (use -ForceNewSecret to regenerate)"
        $clientSecret = $existingCreds.ClientSecret
    } else {
        if ($ForceNewSecret) {
            Write-Warn "Service principal exists - forcing new secret generation"
        } else {
            Write-Warn "Service principal exists but no local credentials found"
        }
        Write-Status "Creating new client secret..."
        
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
    Write-Status "Checking AWS Secrets Manager..."
    
    # Temporarily allow errors for AWS CLI
    $ErrorActionPreference = "Continue"
    
    # Build secret JSON and save to temp file (avoids shell escaping issues)
    $secretObj = @{
        AZURE_TENANT_ID = $tenantId
        AZURE_CLIENT_ID = $clientId
        AZURE_CLIENT_SECRET = $clientSecret
        AZURE_SUBSCRIPTION_ID = $subscriptionId
    }
    $secretValue = $secretObj | ConvertTo-Json -Compress
    $tempFile = [System.IO.Path]::GetTempFileName()
    # Write without BOM (Out-File adds BOM which corrupts the JSON)
    [System.IO.File]::WriteAllText($tempFile, $secretValue)
    
    # Check if secret exists and get current value
    $existingSecret = $null
    $existingParsed = $null
    $checkResult = aws secretsmanager get-secret-value --secret-id $AwsSecretName --region $AwsRegion 2>&1
    if ($LASTEXITCODE -eq 0) {
        try {
            $existingSecret = $checkResult | ConvertFrom-Json
            $secretString = $existingSecret.SecretString
            Write-Host "  [Debug] SecretString from AWS: $secretString" -ForegroundColor Gray
            Write-Host "  [Debug] Local secretValue: $secretValue" -ForegroundColor Gray
            $existingParsed = $secretString | ConvertFrom-Json
            Write-Host "  [Debug] Parsed TenantID: '$($existingParsed.AZURE_TENANT_ID)' vs Local: '$tenantId'" -ForegroundColor Gray
        } catch {
            Write-Host "  [Debug] Parse error: $_" -ForegroundColor Gray
            $existingParsed = $null
        }
    }
    
    if ($existingSecret) {
        # Compare actual credential values (not JSON strings which may have different key ordering)
        $match_tenant = $existingParsed.AZURE_TENANT_ID -eq $tenantId
        $match_client = $existingParsed.AZURE_CLIENT_ID -eq $clientId
        $match_secret = $existingParsed.AZURE_CLIENT_SECRET -eq $clientSecret
        $match_sub = $existingParsed.AZURE_SUBSCRIPTION_ID -eq $subscriptionId
        
        $credsMatch = $existingParsed -and $match_tenant -and $match_client -and $match_secret -and $match_sub
        
        if ($credsMatch) {
            Write-Success "Secret unchanged: $AwsSecretName (skipping update)"
        } else {
            # Show which field changed for debugging
            if (-not $match_tenant) { Write-Host "  [Debug] Tenant ID changed" -ForegroundColor Gray }
            if (-not $match_client) { Write-Host "  [Debug] Client ID changed" -ForegroundColor Gray }
            if (-not $match_secret) { Write-Host "  [Debug] Client Secret changed" -ForegroundColor Gray }
            if (-not $match_sub) { Write-Host "  [Debug] Subscription ID changed" -ForegroundColor Gray }
            
            Write-Status "Credentials changed, updating secret..."
            $updateResult = aws secretsmanager put-secret-value --secret-id $AwsSecretName --secret-string "file://$tempFile" --region $AwsRegion 2>&1
            if ($LASTEXITCODE -eq 0) {
                Write-Success "Updated secret: $AwsSecretName"
            } else {
                Write-Err "Failed to update secret: $updateResult"
            }
        }
    } else {
        Write-Status "Creating new secret..."
        $createResult = aws secretsmanager create-secret --name $AwsSecretName --description "Azure credentials for Spot Analyzer" --secret-string "file://$tempFile" --region $AwsRegion 2>&1
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
Write-Host "For Lambda deployment:"
Write-Host "  1. Ensure AWS Secrets Manager has the Azure credentials"
Write-Host "  2. Run: python utils\lambda\sam_deploy.py"
Write-Host "  3. Lambda will automatically load Azure creds from Secrets Manager"
Write-Host ""
Write-Host "Azure permissions granted (via Reader role):"
Write-Host "  - Microsoft.Compute/skus/read (VM availability)"
Write-Host "  - Microsoft.Compute/locations/*/read (Region info)"
Write-Host "  - Microsoft.Resources/subscriptions/read (Subscription info)"
Write-Host ""

# Security reminder
Write-Warn "SECURITY REMINDER:"
Write-Host "  - azure-config.yaml is already in .gitignore"
Write-Host "  - Never commit credentials to source control"
Write-Host ""
