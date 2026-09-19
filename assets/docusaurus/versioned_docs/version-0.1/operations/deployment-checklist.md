---
title: Production deployment checklist
description: A pre-deployment, deployment, and post-deployment checklist for running the Agentic Identity Broker with AWS-backed encryption infrastructure.
---

# CDK Encryption Infrastructure Deployment Checklist

## Pre-Deployment

### For Kubernetes IRSA Deployments

- [ ] EKS cluster has OIDC provider configured
- [ ] OIDC provider ARN captured for `-c oidcProviderArn=...`
- [ ] OIDC subject key captured for `-c oidcSubjectKey=...` (for EKS: `<issuer-host>:sub`)
- [ ] Kubernetes namespace and service account name defined for `-c serviceAccountSubject=...`
- [ ] Environment set correctly (-c env=prod for production)
- [ ] AWS credentials configured (aws sts get-caller-identity)
- [ ] VPC/network access to KMS and DynamoDB verified
- [ ] IAM permissions to create KMS, DynamoDB, IAM resources
- [ ] CloudFormation template reviewed (cdk diff)
- [ ] Backup/rollback plan documented

## Deployment Steps

### Kubernetes IRSA Deployment (Recommended)

The IRSA pattern uses federated identity via an IAM OIDC provider for secure, credential-free AWS access from Kubernetes pods. CDK deployment requires `oidcProviderArn`, `oidcSubjectKey`, and `serviceAccountSubject` context values.

```bash
# 1. Extract OIDC provider information
OIDC_ISSUER=$(aws eks describe-cluster \
  --name my-cluster \
  --query 'cluster.identity.oidc.issuer' \
  --output text)

OIDC_PROVIDER_HOST=${OIDC_ISSUER#https://}
ACCOUNT_ID=$(aws sts get-caller-identity --query 'Account' --output text)
OIDC_PROVIDER_ARN="arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER_HOST}"
OIDC_SUBJECT_KEY="${OIDC_PROVIDER_HOST}:sub"

# 2. Define service account details
export K8S_NAMESPACE="identity-broker"
export K8S_SERVICE_ACCOUNT="broker-sa"

# 3. Synthesize CloudFormation template (review for errors)
cd infra/cdk
npx cdk synth \
  -c env=prod \
  -c oidcProviderArn="$OIDC_PROVIDER_ARN" \
  -c oidcSubjectKey="$OIDC_SUBJECT_KEY" \
  -c serviceAccountSubject="system:serviceaccount:${K8S_NAMESPACE}:${K8S_SERVICE_ACCOUNT}"

# 4. Preview infrastructure changes
npx cdk diff \
  -c env=prod \
  -c oidcProviderArn="$OIDC_PROVIDER_ARN" \
  -c oidcSubjectKey="$OIDC_SUBJECT_KEY" \
  -c serviceAccountSubject="system:serviceaccount:${K8S_NAMESPACE}:${K8S_SERVICE_ACCOUNT}"

# 5. Deploy with confirmation prompt
npx cdk deploy \
  -c env=prod \
  -c oidcProviderArn="$OIDC_PROVIDER_ARN" \
  -c oidcSubjectKey="$OIDC_SUBJECT_KEY" \
  -c serviceAccountSubject="system:serviceaccount:${K8S_NAMESPACE}:${K8S_SERVICE_ACCOUNT}"
```

## Post-Deployment Verification

- [ ] **KMS key exists:**

  ```bash
  aws kms describe-key --key-id alias/agentic-identity-broker/prod/token-vault-kek
  # Expected: KeyState=Enabled, KeyRotationEnabled=true
  ```

- [ ] **DynamoDB table exists:**

  ```bash
  aws dynamodb describe-table --table-name AgenticIdentityBrokerBranchKeys-prod
  # Expected: TableStatus=ACTIVE
  # Note: PITR is ENABLED only in production, disabled in dev/staging
  # Note: DeletionProtection is ENABLED only in production
  ```

- [ ] **IAM role exists:**

  ```bash
  # IAM role name is created by CDK with the format: AgenticIdentityBrokerEncryptionRole-{env}
  # NOTE: Role name includes "-Role" suffix (different from stack name AgenticIdentityBrokerEncryptionVault-{env})
  aws iam get-role --role-name AgenticIdentityBrokerEncryptionRole-prod
  # Expected: Role exists with KMS and DynamoDB permissions
  # Note: MaxSessionDuration = 1 hour (for temporary credential limitation)
  ```

- [ ] **Extract stack outputs**:

  ```bash
  STACK_NAME="AgenticIdentityBrokerEncryptionVault-prod"

  aws cloudformation describe-stacks \
    --stack-name $STACK_NAME \
    --query 'Stacks[0].Outputs' \
    --output table
  ```

### For Kubernetes IRSA Deployments

- [ ] **IAM role trust policy includes the federated principal:**

  ```bash
  IAM_ROLE_NAME=$(aws cloudformation describe-stacks \
    --stack-name AgenticIdentityBrokerEncryptionVault-prod \
    --query 'Stacks[0].Outputs[?OutputKey==`IamRoleName`].OutputValue' \
    --output text)

  aws iam get-role --role-name $IAM_ROLE_NAME \
    --query 'Role.AssumeRolePolicyDocument' \
    --output json | jq .

  # Expected: Principal.Federated = OIDC provider ARN from oidcProviderArn
  # Expected: Condition.StringEquals maps oidcSubjectKey to serviceAccountSubject
  ```

- [ ] **Extract stack outputs for Helm configuration**:

  ```bash
  # Extract outputs needed for Helm values
  export KMS_KEY_ARN=$(aws cloudformation describe-stacks \
    --stack-name AgenticIdentityBrokerEncryptionVault-prod \
    --query 'Stacks[0].Outputs[?OutputKey==`EncryptionKeyARN`].OutputValue' \
    --output text)

  export DYNAMODB_TABLE_NAME=$(aws cloudformation describe-stacks \
    --stack-name AgenticIdentityBrokerEncryptionVault-prod \
    --query 'Stacks[0].Outputs[?OutputKey==`BranchKeyTableName`].OutputValue' \
    --output text)

  export IAM_ROLE_ARN=$(aws cloudformation describe-stacks \
    --stack-name AgenticIdentityBrokerEncryptionVault-prod \
    --query 'Stacks[0].Outputs[?OutputKey==`EncryptionRoleARN`].OutputValue' \
    --output text)

  # Extract IAM role name for IRSA annotation
  export IAM_ROLE_NAME=$(aws cloudformation describe-stacks \
    --stack-name AgenticIdentityBrokerEncryptionVault-prod \
    --query 'Stacks[0].Outputs[?OutputKey==`IamRoleName`].OutputValue' \
    --output text)

  echo "KMS Key ARN: $KMS_KEY_ARN"
  echo "DynamoDB Table: $DYNAMODB_TABLE_NAME"
  echo "IAM Role ARN: $IAM_ROLE_ARN"
  echo "IAM Role Name: $IAM_ROLE_NAME"

  # View all available stack outputs
  echo -e "\n=== All Stack Outputs ==="
  aws cloudformation describe-stacks \
    --stack-name AgenticIdentityBrokerEncryptionVault-prod \
    --query 'Stacks[0].Outputs[*].[OutputKey,OutputValue]' \
    --output table
  ```

- [ ] **Create Kubernetes namespace**:

  ```bash
  kubectl create namespace ${K8S_NAMESPACE}
  ```

- [ ] **Deploy with Helm (IRSA configured)**:

  ```bash
  helm install broker ./charts/agentic-identity-broker \
    -n ${K8S_NAMESPACE} \
    --set serviceAccount.create=true \
    --set serviceAccount.name=${K8S_SERVICE_ACCOUNT} \
    --set serviceAccount.irsa.enabled=true \
    --set serviceAccount.irsa.role="${IAM_ROLE_ARN}" \
    --set broker.extraConfig.encryption.aws_kms.key_arn="${KMS_KEY_ARN}" \
    --set broker.extraConfig.encryption.aws_kms.dynamodb_table_name="${DYNAMODB_TABLE_NAME}" \
    --set storage.type=postgres \
    --set postgresql.external.enabled=true
  ```

- [ ] **ServiceAccount has the IRSA annotation:**

  ```bash
  kubectl get serviceaccount ${K8S_SERVICE_ACCOUNT} \
    -n ${K8S_NAMESPACE} \
    -o jsonpath='{.metadata.annotations.iam\.amazonaws\.com/role}'

  # Expected: arn:aws:iam::ACCOUNT:role/AgenticIdentityBrokerEncryptionRole-prod
  ```

- [ ] **Pod can assume the IAM role:**

  ```bash
  POD_NAME=$(kubectl get pods -n ${K8S_NAMESPACE} \
    -l app.kubernetes.io/name=agentic-identity-broker \
    --output jsonpath='{.items[0].metadata.name}')

  kubectl exec -n ${K8S_NAMESPACE} $POD_NAME -- env | grep AWS_ROLE_ARN
  kubectl logs -n ${K8S_NAMESPACE} $POD_NAME | grep -i "credential\|kms\|dynamodb"

  # Expected: AWS_ROLE_ARN environment variable set
  # Expected: No credential or permission errors in logs
  ```

- [ ] **Test encryption functionality**:

  ```bash
  # Port-forward to test
  kubectl port-forward -n ${K8S_NAMESPACE} svc/broker-agentic-identity-broker 8000:8000 &

  # Test health
  curl http://localhost:8000/health

  # Test OAuth2 session (tests KMS encryption)
  curl -X GET http://localhost:8000/api/third-party/sessions \
    -H "X-Remote-User: testuser@example.com"
  ```

- [ ] **Run smoke test** (OAuth2 session management):

  ```bash
  # List OAuth2 sessions (verify encryption is working)
  curl -X GET http://localhost:8000/api/third-party/sessions \
    -H "X-Remote-User: testuser@example.com"

  # Verify session details can be retrieved
  curl -X GET http://localhost:8000/api/third-party/{serviceId}/session \
    -H "X-Remote-User: testuser@example.com"
  ```

  **Alternative**: Create a full OAuth2 session flow via `/api/third-party/{serviceId}/oauth2/authorize` and `/api/third-party/{serviceId}/oauth2/callback` endpoints.

- [ ] **CloudWatch alarms exist:**

  ```bash
  aws cloudwatch describe-alarms \
    --alarm-name-prefix AgenticIdentityBroker-Encryption-prod

  # Expected alarms:
  # - AgenticIdentityBroker-Encryption-prod-KMS-Throttle
  # - AgenticIdentityBroker-Encryption-prod-KMS-Errors
  ```

- [ ] **Configure SNS notifications for alarms** (Optional):

  The CDK stack creates CloudWatch alarms but does NOT automatically create SNS topics. To receive notifications, either:

  **Option A: Create SNS topics and link manually**

  ```bash
  # Create SNS topics
  aws sns create-topic --name AgenticIdentityBroker-Encryption-prod-KMS-Alerts

  # Get topic ARN
  TOPIC_ARN=$(aws sns list-topics \
    --query 'Topics[?TopicArn==`*AgenticIdentityBroker-Encryption-prod-KMS-Alerts*`].TopicArn' \
    --output text)

  # Link alarms to SNS topic
  aws cloudwatch put-metric-alarm \
    --alarm-name AgenticIdentityBroker-Encryption-prod-KMS-Throttle \
    --alarm-actions $TOPIC_ARN

  aws cloudwatch put-metric-alarm \
    --alarm-name AgenticIdentityBroker-Encryption-prod-KMS-Errors \
    --alarm-actions $TOPIC_ARN

  # Subscribe to topic
  aws sns subscribe \
    --topic-arn $TOPIC_ARN \
    --protocol email \
    --notification-endpoint ops-team@example.com
  ```

  **Option B: Use AWS Console or other alerting tools**
  - Integrate with PagerDuty, Slack, or other services using CloudWatch integrations
  - Configure alarm actions in AWS Console

## Rollback Procedures

If deployment fails or issues are discovered:

### Option 1: Automatic Rollback (Default)

```bash
# CloudFormation automatically rolls back failed stacks
# No manual action required - monitors stack events
aws cloudformation describe-stack-events \
  --stack-name AgenticIdentityBrokerEncryptionVault-prod
```

### Option 2: Manual Rollback

```bash
# Cancel in-progress deployment
aws cloudformation cancel-update-stack \
  --stack-name AgenticIdentityBrokerEncryptionVault-prod

# Verify stack returns to previous state
aws cloudformation describe-stacks \
  --stack-name AgenticIdentityBrokerEncryptionVault-prod \
  --query 'Stacks[0].StackStatus'
```

### Option 3: Delete Stack (Emergency Only)

```bash
# WARNING: This deletes all encryption infrastructure
# Only use if stack is unrecoverable or in non-production
cd infra/cdk
npx cdk destroy -c env=prod

# Note: KMS keys have 30-day pending deletion window
# They can be recovered during this period if needed
```

## Infrastructure Details

### KMS Key Deletion Window

The CDK stack configures KMS key deletion windows based on environment:

- **Production**: 30-day pending deletion window (maximum safety)
- **Non-production (dev/staging)**: 7-day pending deletion window

This means if you delete the KMS key:

- In production: You have 30 days to recover it before permanent deletion
- In non-production: You have 7 days to recover it before permanent deletion

To recover a key during the deletion window:

```bash
aws kms cancel-key-deletion --key-id alias/agentic-identity-broker/prod/token-vault-kek
```

### DynamoDB Table Features

- **Billing Mode**: Pay-per-request (automatic scaling, no provisioned capacity)
- **Encryption**: AWS-managed encryption by default
- **Point-in-Time Recovery (PITR)**: **Enabled for production only**
- **Deletion Protection**: **Enabled for production only**

To enable PITR or deletion protection for non-production:

```bash
# Enable PITR
aws dynamodb update-continuous-backups \
  --table-name AgenticIdentityBrokerBranchKeys-prod \
  --point-in-time-recovery-specification PointInTimeRecoveryEnabled=true

# Enable deletion protection
aws dynamodb update-table \
  --table-name AgenticIdentityBrokerBranchKeys-prod \
  --deletion-protection-enabled
```

### IAM Role Configuration

- **Trust Policy**: Uses IRSA (IAM Roles for Service Accounts) for Kubernetes
- **Session Duration**: 1 hour maximum (enforced for temporary credential safety)
- **Permissions**: Least-privilege KMS and DynamoDB operations
- **KMS Permissions**: encrypt, decrypt, generate data keys, describe key, create grants
- **DynamoDB Permissions**: get/put/query/update/delete item, describe table

---

## AWS KMS Configuration Options

All AWS KMS configuration can be set via environment variables:

### Key Encryption Setup

- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN` - **Required**. KMS CMK ARN for envelope encryption (from CDK stack output)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME` - DynamoDB table name for branch key caching (from CDK stack output)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL` - TTL for cached branch keys (default: "1h")

### DynamoDB Configuration

- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_REGION` - AWS region for DynamoDB operations
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TIMEOUT` - Per-operation timeout covering encrypt/decrypt and branch-key operations (optional)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_ENDPOINT` - Custom DynamoDB endpoint (for AWS emulator testing)

### AWS SDK Configuration

- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_REGION` - AWS region for KMS operations
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ENDPOINT` - Custom KMS endpoint URL (for AWS emulator testing)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_PROFILE` - AWS profile for credentials (~/.aws/credentials)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ACCESS_KEY_ID` - Static AWS access key ID (for CI/testing)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_SECRET_ACCESS_KEY` - Static AWS secret access key (for CI/testing)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ASSUME_ROLE_ARN` - IAM role ARN to assume for operations (from CDK stack output)
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DISABLE_SSL` - Disable SSL verification (development only, never in production)

## Success Criteria

### Before Production Deployment

- [ ] All HIGH priority fixes implemented and tested
- [ ] `serviceAccountSubject`, `oidcProviderArn`, and `oidcSubjectKey` enforced (panic if missing)
- [ ] Environment validation prevents typos (dev/staging/prod only)
- [ ] Test KMS key rotation and token decryption
- [ ] Security review completed
- [ ] Load testing performed

### Before General Availability (GA) Release

- [ ] All MEDIUM priority fixes implemented
- [ ] CloudWatch alarms configured and monitored
- [ ] Operational runbooks complete and tested
- [ ] Cost tracking and budgets configured
- [ ] Disaster recovery procedures tested
- [ ] On-call team trained on operations
- [ ] Documentation published and accessible

## Monitoring and Alerting

After deployment, monitor these resources continuously:

```bash
# View CloudWatch dashboard
aws cloudwatch get-dashboard \
  --dashboard-name AgenticIdentityBroker-Encryption-prod

# Monitor KMS API call rate (matches dashboard metric)
aws cloudwatch get-metric-statistics \
  --namespace AWS/KMS \
  --metric-name ApiCalls \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Sum \
  --dimensions Name=KeyId,Value=<KEY_ID>

# Monitor DynamoDB read/write capacity
aws cloudwatch get-metric-statistics \
  --namespace AWS/DynamoDB \
  --metric-name ConsumedReadCapacityUnits \
  --dimensions Name=TableName,Value=AgenticIdentityBrokerBranchKeys-prod \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Sum
```

## Cost Estimation

Expected monthly costs for production:

- **KMS Key**: $1/month + $0.03 per 10,000 requests
- **DynamoDB Table**: Pay-per-request pricing (~$0.25 per million reads)
- **DynamoDB PITR**: ~$0.20 per GB-month of table size
- **CloudWatch Alarms**: $0.10 per alarm per month
- **CloudWatch Dashboard**: $3/month per dashboard

**Total estimated cost**: $10-50/month depending on request volume

## CloudFormation Stack Outputs Reference

All outputs from the CDK stack `AgenticIdentityBrokerEncryptionVault-{env}`:

> **Important**: The stack name is `AgenticIdentityBrokerEncryptionVault-{env}`, but the IAM role name is `AgenticIdentityBrokerEncryptionRole-{env}` (includes "-Role" suffix). Use the correct name when calling `aws iam get-role --role-name`.

| Output Key | Description | Usage |
|------------|-------------|-------|
| `EncryptionKeyARN` | KMS CMK ARN | → `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN` |
| `EncryptionKeyAlias` | KMS key alias | Human-readable reference: `alias/agentic-identity-broker/{env}/token-vault-kek` |
| `BranchKeyTableName` | DynamoDB table name | → `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME` |
| `BranchKeyTableARN` | DynamoDB table ARN | IAM policy reference |
| `EncryptionRoleARN` | IAM role ARN | Cross-account access, role assumption |
| `IamRoleName` | IAM role name (`AgenticIdentityBrokerEncryptionRole-{env}`) | IRSA annotation: `iam.amazonaws.com/role={IamRoleName}` |
| `KMSThrottleAlarmArn` | CloudWatch alarm (KMS throttling) | Link to SNS for alerts |
| `KMSErrorAlarmArn` | CloudWatch alarm (KMS errors) | Link to SNS for alerts |

---

## References

- [Kubernetes IRSA Deployment Guide](../deployment/kubernetes-irsa.md) - Complete IRSA deployment walkthrough
- [AWS CDK Deployment Guide](https://docs.aws.amazon.com/cdk/latest/guide/home.html)
- [CloudFormation Stack Operations](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-console-view-stack-data-resources.html)
- [EKS IAM Roles for Service Accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
- [AWS Encryption SDK Hierarchical Keyring](https://docs.aws.amazon.com/encryption-sdk/latest/developer-guide/use-hierarchical-keyring.html)
