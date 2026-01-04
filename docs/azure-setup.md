# Azure Setup Guide

This guide covers setting up Azure integration for Spot Analyzer, enabling smart availability zone recommendations based on real Azure data.

## Overview

Spot Analyzer supports two levels of Azure integration:

| Level | Authentication | Features |
|-------|---------------|----------|
| **Basic** | None required | Spot prices, savings analysis, instance recommendations |
| **Full** | Service Principal | All basic + per-zone VM availability, smart AZ recommendations |

## Basic Setup (No Authentication)

Azure Retail Prices API is public and requires no authentication. You can use Spot Analyzer with Azure immediately:

```powershell
cd c:\spot-analyzer
.\spot-web.exe
```

Select "Azure" in the UI and analyze instances. Basic features work out of the box.

## Full Setup (With Authentication)

For smart AZ recommendations based on real zone availability data, you need Azure credentials.

### Prerequisites

1. **Azure CLI** - Install if not present:
   ```powershell
   winget install Microsoft.AzureCLI
   ```
   Restart your terminal after installation.

2. **Azure Subscription** - You need at least Reader access to a subscription.

3. **AWS CLI** (optional) - Only needed if you want to store credentials in AWS Secrets Manager for Lambda deployment.

### Quick Setup (Recommended)

Run the automated setup script:

```powershell
cd c:\spot-analyzer\utils\azure
.\setup_azure_creds.ps1
```

This script will:
1. Log you into Azure (opens browser)
2. Create a service principal named "spot-analyzer" (or reuse existing)
3. Save credentials to `config.yaml`
4. Optionally create an AWS Secrets Manager secret for Lambda

### Manual Setup

If you prefer manual setup:

#### Step 1: Login to Azure
```powershell
az login
```

#### Step 2: Create Service Principal
```powershell
# Get your subscription ID
az account show --query id -o tsv

# Create service principal with Reader role
az ad sp create-for-rbac --name "spot-analyzer" --role "Reader" --scopes /subscriptions/YOUR_SUBSCRIPTION_ID
```

Output:
```json
{
  "appId": "12345678-abcd-...",      // CLIENT_ID
  "password": "abc123...",            // CLIENT_SECRET
  "tenant": "87654321-dcba-..."       // TENANT_ID
}
```

#### Step 3: Add to config.yaml

Add the following to your `config.yaml`:

```yaml
azure:
  tenantId: "YOUR_TENANT_ID"
  clientId: "YOUR_CLIENT_ID"
  clientSecret: "YOUR_CLIENT_SECRET"
  subscriptionId: "YOUR_SUBSCRIPTION_ID"
```

## Configuration Options

### config.yaml (Local Development)

```yaml
azure:
  # Public API settings (no auth required)
  retail_prices_url: "https://prices.azure.com/api/retail/prices"
  default_region: "eastus"
  http_timeout: 60s
  
  # Authentication (for Compute SKUs API)
  tenantId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  clientId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  clientSecret: "your-client-secret"
  subscriptionId: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
```

### Environment Variables

Environment variables override config.yaml settings:

```powershell
$env:AZURE_TENANT_ID = "your-tenant-id"
$env:AZURE_CLIENT_ID = "your-client-id"
$env:AZURE_CLIENT_SECRET = "your-client-secret"
$env:AZURE_SUBSCRIPTION_ID = "your-subscription-id"
```

### AWS Secrets Manager (Lambda Deployment)

For Lambda, credentials are loaded from AWS Secrets Manager:

1. Create the secret:
   ```powershell
   aws secretsmanager create-secret --name "spot-analyzer/azure-credentials" --secret-string '{
     "AZURE_TENANT_ID": "your-tenant-id",
     "AZURE_CLIENT_ID": "your-client-id",
     "AZURE_CLIENT_SECRET": "your-client-secret",
     "AZURE_SUBSCRIPTION_ID": "your-subscription-id"
   }'
   ```

2. The Lambda function automatically reads from this secret. The IAM policy is already configured in `template.yaml`.

## How Azure AZ Recommendations Work

### The Challenge

Unlike AWS, Azure's Retail Prices API does **not** provide per-availability-zone spot pricing. Azure spot prices are the same across all zones in a region.

### Our Smart Solution

We use a **dual-approach analysis** combining real-time Azure APIs with capacity scoring:

#### Approach 1: SKU Availability API

Uses the **Azure Compute Resource SKUs API** to check real-time availability:

```
GET https://management.azure.com/subscriptions/{sub}/providers/Microsoft.Compute/skus
```

This API provides:
- **Zone Availability** - Which zones support the VM size
- **Zone Restrictions** - Capacity constraints, quota limits, or NotAvailableForSubscription
- **Capability Information** - VM capabilities and features per zone

#### Approach 2: Zone Capacity Score

Analyzes VM type diversity per zone to estimate capacity:

1. **Count VM Types** - How many different VM sizes are available in each zone
2. **Normalize to Score** - Convert to 0-100 scale (higher = more capacity)
3. **Indicate Stability** - Zones with more VM types are typically more stable

Example output:
```
Zone capacity scores for eastus: map[eastus-1:25 eastus-2:54 eastus-3:100]
```
In this example, `eastus-3` has 100% score (most VM types), indicating highest capacity.

### Combined Smart Score

Both approaches are combined into a weighted score:

| Factor | Weight | Description |
|--------|--------|-------------|
| Availability | 25% | Is the VM available in this zone? |
| Capacity | 25% | Zone Capacity Score from VM type diversity |
| Price | 20% | Price competitiveness (uniform for Azure) |
| Stability | 15% | Based on restrictions and quota limits |
| Interruption | 15% | Estimated interruption rate |

**Final Score** = (Availability × 0.25) + (Capacity × 0.25) + (Price × 0.20) + (Stability × 0.15) + (Interruption × 0.15)

### UI Display

The smart AZ analysis is displayed in the Web UI with:

- **Score Bar** - Visual representation of the combined score
- **Capacity Badge** - High/Medium/Low capacity indicator
- **Capacity Score** - Numeric score (0-100)
- **Price** - With `~` prefix for predicted prices
- **Interruption Rate** - Estimated interruption percentage
- **Confidence Badge** - High/Medium/Low confidence in the analysis
- **Insights** - Smart recommendations and observations
- **Data Sources** - Which APIs contributed to the analysis

### Example Response Interpretation

| VM Size | Zone 1 | Zone 2 | Zone 3 | Smart Score | Recommendation |
|---------|--------|--------|--------|-------------|----------------|
| Standard_D4s_v5 | ✅ Cap:25 | ✅ Cap:54 | ❌ Restricted | Z2: 87 | Zone 2 (highest capacity) |
| Standard_E8s_v5 | ✅ Cap:25 | ✅ Cap:54 | ✅ Cap:100 | Z3: 95 | Zone 3 (best overall) |
| Standard_M128s | ✅ Cap:25 | ❌ N/A | ❌ N/A | Z1: 45 | Zone 1 only |

## Security Best Practices

1. **Never commit credentials** - Add `config.yaml` to `.gitignore`
2. **Use minimum permissions** - Reader role is sufficient
3. **Rotate secrets regularly** - Use `az ad sp credential reset`
4. **Use Managed Identity** when running on Azure (no credentials needed)

## Troubleshooting

### "Azure credentials not configured"
- Check if `config.yaml` has the azure section
- Verify environment variables are set
- Run `az login` to refresh CLI credentials

### "Failed to get access token"
- Verify client secret is correct and not expired
- Check tenant ID matches your Azure AD directory
- Ensure service principal has Reader role

### "VM size not found in region"
- The VM size may not be available in that region
- Check Azure's region availability: https://azure.microsoft.com/regions/services/

### Lambda not loading Azure credentials
- Verify the secret exists: `aws secretsmanager describe-secret --secret-id spot-analyzer/azure-credentials`
- Check Lambda has IAM permission for `secretsmanager:GetSecretValue`
- Verify the secret JSON format matches expected structure

## API Reference

### Azure Retail Prices API (No Auth)
- **Endpoint**: `https://prices.azure.com/api/retail/prices`
- **Auth**: None
- **Data**: Current spot and on-demand prices per region
- **Limitations**: No per-AZ pricing, no historical data

### Azure Compute SKUs API (Requires Auth)
- **Endpoint**: `https://management.azure.com/subscriptions/{sub}/providers/Microsoft.Compute/skus`
- **Auth**: Bearer token (OAuth 2.0)
- **Data**: VM availability per zone, restrictions, capabilities
- **Scope**: Reader role on subscription

## See Also

- [Main README](../README.md)
- [AWS Setup](./aws-setup.md)
- [Web UI Guide](./web-ui.md)
