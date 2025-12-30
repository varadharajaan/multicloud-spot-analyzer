# Lambda Deployment Utilities

This folder contains utility scripts for deploying and managing the Spot Analyzer Lambda function.

## 🚀 Quick Deploy (Recommended)

Use the direct deployment script for **FREE Function URLs** (no API Gateway costs):

```bash
python utils/lambda/deploy_direct.py --region us-east-1
```

This will:
1. Build the Go Lambda binary
2. Create IAM role with required permissions  
3. Deploy the Lambda function
4. Create a FREE Function URL (like `https://xxx.lambda-url.us-east-1.on.aws/`)

**Cost**: $0 for the URL - you only pay for Lambda invocations!

## Prerequisites

- AWS CLI configured with credentials
- AWS SAM CLI installed (`pip install aws-sam-cli`)
- boto3 installed (`pip install boto3`)
- Go 1.23+ for building the Lambda binary

## Scripts

### `deploy_direct.py` - Direct Deploy (Recommended)

Deploys Lambda with FREE Function URL, bypassing CloudFormation:

```bash
# Default deployment
python utils/lambda/deploy_direct.py

# Custom settings
python utils/lambda/deploy_direct.py --function-name my-analyzer --region us-west-2
```

### `sam_deploy.py` - SAM Deploy (Alternative)

Build and deploy the SAM stack:

```bash
# Build + Deploy (default)
python utils/lambda/sam_deploy.py

# Build only
python utils/lambda/sam_deploy.py -b

# Deploy only (uses previous build)
python utils/lambda/sam_deploy.py -d

# Custom region and stack name
python utils/lambda/sam_deploy.py --region us-west-2 --stack-name my-spot-analyzer
```

### `show_stack_outputs.py` - View Stack Outputs

Display CloudFormation stack outputs (URLs, ARNs):

```bash
# Show outputs for default stack
python utils/lambda/show_stack_outputs.py

# Custom stack/region
python utils/lambda/show_stack_outputs.py --stack spot-analyzer-stack --region us-west-2

# JSON output
python utils/lambda/show_stack_outputs.py --json
```

### `tail_logs.py` - Tail Lambda Logs

Tail CloudWatch logs in real-time:

```bash
# Tail prod logs (default)
python utils/lambda/tail_logs.py

# Tail dev environment
python utils/lambda/tail_logs.py --env dev

# Show last 30 minutes
python utils/lambda/tail_logs.py --since 30m

# Show last hour without following
python utils/lambda/tail_logs.py --since 1h --no-follow

# List available log groups
python utils/lambda/tail_logs.py --list
```

## Quick Start

1. **Deploy the stack:**
   ```bash
   python utils/lambda/sam_deploy.py
   ```

2. **Get the Function URL:**
   ```bash
   python utils/lambda/show_stack_outputs.py
   ```

3. **Test the endpoint:**
   ```bash
   curl https://your-function-url.lambda-url.us-east-1.on.aws/api/health
   ```

4. **Watch logs:**
   ```bash
   python utils/lambda/tail_logs.py
   ```

## Configuration

Edit the scripts to change defaults:

| Script | Variable | Default |
|--------|----------|---------|
| `sam_deploy.py` | `STACK_NAME` | `spot-analyzer-stack` |
| `sam_deploy.py` | `REGION` | `us-east-1` |
| `show_stack_outputs.py` | `DEFAULT_STACK_NAME` | `spot-analyzer-stack` |
| `tail_logs.py` | `DEFAULT_REGION` | `us-east-1` |
| `tail_logs.py` | `DEFAULT_ENV` | `prod` |

## Output Files

- `stack-outputs.txt` - Saved stack outputs after deployment

## Troubleshooting

### SAM CLI not found
```bash
pip install aws-sam-cli
```

### boto3 not found
```bash
pip install boto3
```

### No credentials
```bash
aws configure
```

### Log group not found
The Lambda function needs to be invoked at least once to create the log group.