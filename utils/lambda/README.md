# Lambda Deployment Utilities

This folder contains utility scripts for deploying and managing the Spot Analyzer Lambda function.

## 🚀 Quick Deploy (Recommended)

Use the SAM deployment script for a fully managed CloudFormation deployment:

```bash
python utils/lambda/sam_deploy.py
```

This will:
1. Build the Go Lambda binary for Linux (amd64)
2. Run SAM build to package the function
3. Deploy the CloudFormation stack
4. Create a **FREE** Lambda Function URL (like `https://xxx.lambda-url.us-east-1.on.aws/`)
5. Create CloudWatch Log Group with 14-day retention
6. Save stack outputs to `stack-outputs.txt`
7. **Auto-update README.md** with the new Function URL

**Cost**: $0 for the URL - you only pay for Lambda invocations!

## Prerequisites

- AWS CLI configured with credentials
- AWS SAM CLI installed (`pip install aws-sam-cli`)
- boto3 installed (`pip install boto3`)
- Go 1.23+ for building the Lambda binary

## Scripts

### `sam_deploy.py` - Build & Deploy

Build and deploy the SAM stack:

```bash
# Build + Deploy (default) - also updates README.md
python utils/lambda/sam_deploy.py

# Build only
python utils/lambda/sam_deploy.py -b

# Deploy only (uses previous build)
python utils/lambda/sam_deploy.py -d

# Skip README.md auto-update
python utils/lambda/sam_deploy.py --no-readme-update

# Custom region and stack name
python utils/lambda/sam_deploy.py --region us-west-2 --stack-name my-spot-analyzer
```

### `sam_cleanup.py` - Full Stack Cleanup

Delete the stack and all associated resources:

```bash
# Cleanup prod stack (default)
python utils/lambda/sam_cleanup.py

# Cleanup dev stack
python utils/lambda/sam_cleanup.py --env dev

# Cleanup ALL spot-analyzer stacks
python utils/lambda/sam_cleanup.py --all

# Dry run - see what would be deleted
python utils/lambda/sam_cleanup.py --dry-run

# Skip confirmation prompt
python utils/lambda/sam_cleanup.py -y
```

### `show_stack_outputs.py` - View Stack Outputs

Display CloudFormation stack outputs (URLs, ARNs):

```bash
# Show outputs for default stack
python utils/lambda/show_stack_outputs.py

# Custom stack/region
python utils/lambda/show_stack_outputs.py --stack spot-analyzer-prod --region us-west-2

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

5. **Cleanup when done:**
   ```bash
   python utils/lambda/sam_cleanup.py
   ```

## Configuration

Default values in the scripts:

| Script | Variable | Default |
|--------|----------|---------|
| `sam_deploy.py` | `STACK_NAME` | `spot-analyzer-prod` |
| `sam_deploy.py` | `REGION` | `us-east-1` |
| `sam_cleanup.py` | `DEFAULT_STACK_NAME` | `spot-analyzer-prod` |
| `show_stack_outputs.py` | `DEFAULT_STACK_NAME` | `spot-analyzer-prod` |
| `tail_logs.py` | `DEFAULT_STACK_NAME` | `spot-analyzer-prod` |

## SAM Template Features

The `template.yaml` includes:

- **Lambda Function URL** - Public endpoint with no IAM auth required
- **CloudWatch Log Group** - 14-day log retention (configurable)
- **IAM Policies** - EC2 permissions for spot price data
- **Environment Parameter** - Deploy as `dev` or `prod`

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

### Log group already exists error
If you see `AWS::EarlyValidation::ResourceExistenceCheck` error, delete the existing log group:
```bash
aws logs delete-log-group --log-group-name /aws/lambda/spot-analyzer-prod --region us-east-1
```

Or use the cleanup script to fully reset:
```bash
python utils/lambda/sam_cleanup.py -y
python utils/lambda/sam_deploy.py
```

### Log group not found for tail
The Lambda function needs to be invoked at least once to create logs.
