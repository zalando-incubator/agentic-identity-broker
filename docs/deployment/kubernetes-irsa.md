---
title: Set up AWS IRSA access
description: Grant broker pods access to AWS KMS and DynamoDB on Amazon EKS using IAM Roles for Service Accounts (IRSA), without static credentials.
---

# Kubernetes IRSA Deployment Guide

This guide covers deploying the Agentic Identity Broker to Amazon EKS using IAM Roles for Service Accounts (IRSA) for secure AWS resource access.

## Overview

IRSA (IAM Roles for Service Accounts) enables Kubernetes pods to assume AWS IAM roles without requiring static credentials. This integration provides:

- **Least privilege access**: Each service account gets its own IAM role
- **No credential management**: No AWS keys stored in Kubernetes Secrets
- **Audit trail**: CloudTrail logs show which pods accessed AWS services
- **Automatic credential rotation**: AWS SDK handles credential refresh
- **Fine-grained permissions**: KMS and DynamoDB access scoped to specific resources

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         EKS Cluster                         │
│                                                             │
│  ┌──────────────────────────────────────────────────┐     │
│  │  Pod: agentic-identity-broker                    │     │
│  │                                                   │     │
│  │  ServiceAccount: broker-sa                       │     │
│  │  Annotation: iam.amazonaws.com/role=RoleARN      │     │
│  │                                                   │     │
│  │  AWS SDK → STS AssumeRoleWithWebIdentity        │     │
│  └──────────────────┬───────────────────────────────┘     │
│                     │                                       │
│                     │ Federated Trust (OIDC)              │
└─────────────────────┼───────────────────────────────────────┘
                      │
                      ▼
         ┌────────────────────────┐
         │  AWS IAM Role         │
         │  (Created by CDK)     │
         │                       │
         │  Trust Policy:        │
         │  - OIDC Provider      │
         │  - Namespace          │
         │  - Service Account    │
         │                       │
         │  Permissions:         │
         │  - KMS Decrypt        │
         │  - DynamoDB R/W       │
         └───────┬───────────────┘
                 │
                 ▼
    ┌────────────────────────────┐
    │  Encryption Resources      │
    │  - KMS CMK                 │
    │  - DynamoDB Table          │
    └────────────────────────────┘
```

## Prerequisites

Before you deploy with IRSA, make sure that these prerequisites are available:

### 1. EKS Cluster with OIDC Provider

Your EKS cluster must have an OIDC identity provider configured:

```bash
# Check if OIDC provider exists
aws eks describe-cluster \
  --name my-cluster \
  --query 'cluster.identity.oidc.issuer' \
  --output text

# Example output:
# https://oidc.eks.us-east-1.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE
```

If not configured, create the OIDC provider:

```bash
eksctl utils associate-iam-oidc-provider \
  --cluster my-cluster \
  --approve
```

### 2. Get OIDC Provider ARN and Subject Key

Extract the OIDC provider ARN and subject condition key for CDK deployment:

```bash
# Get cluster OIDC issuer
OIDC_ISSUER=$(aws eks describe-cluster \
  --name my-cluster \
  --query 'cluster.identity.oidc.issuer' \
  --output text)

# Derive provider host from the issuer URL
OIDC_PROVIDER_HOST=${OIDC_ISSUER#https://}

# Get AWS account ID
ACCOUNT_ID=$(aws sts get-caller-identity --query 'Account' --output text)

# Construct deploy-time context values
OIDC_PROVIDER_ARN="arn:aws:iam::${ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER_HOST}"
OIDC_SUBJECT_KEY="${OIDC_PROVIDER_HOST}:sub"

echo "OIDC Provider ARN: $OIDC_PROVIDER_ARN"
echo "OIDC Subject Key: $OIDC_SUBJECT_KEY"
```

These values are required alongside `serviceAccountSubject` for every `cdk synth`, `cdk diff`, and `cdk deploy` invocation.

### 3. Define Service Account Details

Choose your Kubernetes namespace and service account name:

```bash
export K8S_NAMESPACE="identity-broker"
export K8S_SERVICE_ACCOUNT="broker-sa"
```

### 4. Tools Required

- AWS CLI 2.x
- Go 1.27.1+ (CDK infrastructure is written in Go)
- Node.js 24+ with AWS CDK CLI (`npm install -g aws-cdk`)
- kubectl (configured for your EKS cluster)
- Helm 3.x
- jq (for JSON parsing)

**Note:** The CDK infrastructure is written in Go but uses the Node.js AWS CDK CLI to synthesize and deploy. The CLI invokes the Go code as configured in `cdk.json`.

## Deployment Steps

### Step 1: Deploy CDK Encryption Infrastructure

The CDK stack creates:
- KMS CMK for envelope encryption
- DynamoDB table for branch key caching
- IAM role with IRSA trust policy
- CloudWatch dashboard and alarms

#### 1.1 Navigate to CDK Directory

```bash
cd infra/cdk
```

#### 1.2 Install CDK Dependencies

The CDK infrastructure uses a Go/Node.js hybrid approach: the infrastructure code is written in Go, but deployed via the AWS CDK CLI (Node.js). Install both:

```bash
# Install Go dependencies
go mod tidy && go mod download

# Install AWS CDK CLI globally (or use npx if preferred)
npm install -g aws-cdk
# Alternative (if you don't want to install globally):
# npx aws-cdk@latest (then use npx cdk instead of cdk in subsequent commands)
```

#### 1.3 Synthesize CloudFormation Template

The AWS CDK CLI will invoke the Go code to generate a CloudFormation template:

```bash
npx cdk synth \
  -c env=prod \
  -c oidcProviderArn="$OIDC_PROVIDER_ARN" \
  -c oidcSubjectKey="$OIDC_SUBJECT_KEY" \
  -c serviceAccountSubject="system:serviceaccount:${K8S_NAMESPACE}:${K8S_SERVICE_ACCOUNT}"
```

This command:
1. Calls the Go CDK app (via `go run .` as configured in `cdk.json`)
2. Generates a CloudFormation template in `cdk.out/`

Review the synthesized template: `cdk.out/AgenticIdentityBrokerEncryptionVault-prod.template.json`

#### 1.4 Preview Infrastructure Changes

```bash
npx cdk diff \
  -c env=prod \
  -c oidcProviderArn="$OIDC_PROVIDER_ARN" \
  -c oidcSubjectKey="$OIDC_SUBJECT_KEY" \
  -c serviceAccountSubject="system:serviceaccount:${K8S_NAMESPACE}:${K8S_SERVICE_ACCOUNT}"
```

#### 1.5 Deploy Stack

```bash
npx cdk deploy \
  -c env=prod \
  -c oidcProviderArn="$OIDC_PROVIDER_ARN" \
  -c oidcSubjectKey="$OIDC_SUBJECT_KEY" \
  -c serviceAccountSubject="system:serviceaccount:${K8S_NAMESPACE}:${K8S_SERVICE_ACCOUNT}"
```

Deployment takes approximately 3-5 minutes.

### Step 2: Extract Stack Outputs

After successful deployment, extract the stack outputs needed for Helm configuration:

```bash
STACK_NAME="AgenticIdentityBrokerEncryptionVault-prod"

# Extract all outputs
aws cloudformation describe-stacks \
  --stack-name $STACK_NAME \
  --query 'Stacks[0].Outputs' \
  --output table

# Extract specific outputs for Helm
export KMS_KEY_ARN=$(aws cloudformation describe-stacks \
  --stack-name $STACK_NAME \
  --query 'Stacks[0].Outputs[?OutputKey==`EncryptionKeyARN`].OutputValue' \
  --output text)

export DYNAMODB_TABLE_NAME=$(aws cloudformation describe-stacks \
  --stack-name $STACK_NAME \
  --query 'Stacks[0].Outputs[?OutputKey==`BranchKeyTableName`].OutputValue' \
  --output text)

export IAM_ROLE_ARN=$(aws cloudformation describe-stacks \
  --stack-name $STACK_NAME \
  --query 'Stacks[0].Outputs[?OutputKey==`EncryptionRoleARN`].OutputValue' \
  --output text)

export IAM_ROLE_NAME=$(aws cloudformation describe-stacks \
  --stack-name $STACK_NAME \
  --query 'Stacks[0].Outputs[?OutputKey==`IamRoleName`].OutputValue' \
  --output text)

# Verify outputs
echo "KMS Key ARN: $KMS_KEY_ARN"
echo "DynamoDB Table: $DYNAMODB_TABLE_NAME"
echo "IAM Role ARN: $IAM_ROLE_ARN"
echo "IAM Role Name: $IAM_ROLE_NAME"
```

Expected outputs:
- `EncryptionKeyARN`: KMS key ARN (e.g., `arn:aws:kms:us-east-1:ACCOUNT:key/UUID`)
- `BranchKeyTableName`: DynamoDB table name (e.g., `AgenticIdentityBrokerBranchKeys-prod`)
- `EncryptionRoleARN`: IAM role ARN (e.g., `arn:aws:iam::ACCOUNT:role/AgenticIdentityBrokerEncryptionRole-prod`)
- `IamRoleName`: IAM role name (e.g., `AgenticIdentityBrokerEncryptionRole-prod`)

### Step 3: Examine IAM role trust policy

Make sure that the IAM role has the required IRSA trust policy:

```bash
aws iam get-role \
  --role-name $IAM_ROLE_NAME \
  --query 'Role.AssumeRolePolicyDocument' \
  --output json | jq .
```

Expected trust policy structure:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::ACCOUNT:oidc-provider/oidc.eks.REGION.amazonaws.com/id/EXAMPLEID"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "oidc.eks.REGION.amazonaws.com/id/EXAMPLEID:sub": "system:serviceaccount:identity-broker:broker-sa",
          "oidc.eks.REGION.amazonaws.com/id/EXAMPLEID:aud": "sts.amazonaws.com"
        }
      }
    }
  ]
}
```

### Step 4: Configure Helm Values for IRSA

Create a Helm values file (`values-irsa.yaml`) with IRSA configuration:

```yaml
# Storage configuration
storage:
  type: postgres  # Use PostgreSQL for production

# PostgreSQL configuration (use external or operator)
postgresql:
  external:
    enabled: true
    host: postgres.database.svc.cluster.local
    port: 5432
    database: broker
    migrationSecretName: broker-db-migration
    brokerSecretName: broker-db

# ServiceAccount configuration with IRSA
serviceAccount:
  create: true
  name: broker-sa
  annotations: {}

  # IRSA configuration
  irsa:
    enabled: true
    role: "arn:aws:iam::ACCOUNT:role/AgenticIdentityBrokerEncryptionRole-prod"  # Replace with actual ARN or role name

# Broker configuration (AWS encryption settings)
broker:
  # Additional configuration passed to config.yaml
  extraConfig:
    encryption:
      aws_kms:
        key_arn: "arn:aws:kms:us-east-1:ACCOUNT:key/UUID"  # Replace with actual ARN
        dynamodb_table_name: "AgenticIdentityBrokerBranchKeys-prod"  # Replace with actual table name
        region: "us-east-1"  # Replace with your region
        branch_key_ttl: "1h"

# Production resource configuration
replicaCount: 3

resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 512Mi

# Autoscaling
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

# Pod disruption budget
podDisruptionBudget:
  enabled: true
  minAvailable: 2

# Ingress configuration (adjust for your environment)
ingress:
  enduser:
    enabled: true
    className: nginx
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: broker-tls
        hosts:
          - broker.example.com

  admin:
    enabled: true
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/whitelist-source-range: "10.0.0.0/8"
    hosts:
      - host: broker-admin.internal.example.com
        paths:
          - path: /
            pathType: Prefix
```

**Automated Configuration**:

You can generate this file automatically using the extracted outputs:

```bash
cat > values-irsa.yaml <<EOF
storage:
  type: postgres

postgresql:
  external:
    enabled: true
    host: postgres.database.svc.cluster.local
    port: 5432
    database: broker
    migrationSecretName: broker-db-migration
    brokerSecretName: broker-db

serviceAccount:
  create: true
  name: ${K8S_SERVICE_ACCOUNT}
  irsa:
    enabled: true
    role: "${IAM_ROLE_ARN}"

broker:
  extraConfig:
    encryption:
      aws_kms:
        key_arn: "${KMS_KEY_ARN}"
        dynamodb_table_name: "${DYNAMODB_TABLE_NAME}"
        region: "us-east-1"

replicaCount: 3

resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 512Mi

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
EOF
```

### Step 5: Create Kubernetes Namespace

Create the namespace specified in the CDK deployment:

```bash
kubectl create namespace ${K8S_NAMESPACE}
```

### Step 6: Create PostgreSQL Secrets

Create database credentials (if using external PostgreSQL):

```bash
kubectl create secret generic broker-db-migration \
  -n ${K8S_NAMESPACE} \
  --from-literal=username=broker-migration \
  --from-literal=password=SECURE_MIGRATION_PASSWORD

kubectl create secret generic broker-db \
  -n ${K8S_NAMESPACE} \
  --from-literal=username=broker \
  --from-literal=password=SECURE_BROKER_PASSWORD
```

### Step 7: Deploy with Helm

Deploy the broker with IRSA configuration:

```bash
cd charts/agentic-identity-broker

helm install broker . \
  -n ${K8S_NAMESPACE} \
  -f values-irsa.yaml
```

### Step 8: Examine deployment

#### 8.1 ServiceAccount annotation

Make sure that the ServiceAccount has the IRSA role annotation:

```bash
kubectl get serviceaccount ${K8S_SERVICE_ACCOUNT} \
  -n ${K8S_NAMESPACE} \
  -o yaml
```

Expected output:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  annotations:
    iam.amazonaws.com/role: arn:aws:iam::ACCOUNT:role/AgenticIdentityBrokerEncryptionRole-prod
  name: broker-sa
  namespace: identity-broker
```

#### 8.2 Pod status

Make sure that broker pods are running:

```bash
kubectl get pods -n ${K8S_NAMESPACE} -l app.kubernetes.io/name=agentic-identity-broker
```

Expected output:

```
NAME                                         READY   STATUS    RESTARTS   AGE
broker-agentic-identity-broker-xxxxx-yyyyy   1/1     Running   0          2m
broker-agentic-identity-broker-xxxxx-zzzzz   1/1     Running   0          2m
broker-agentic-identity-broker-xxxxx-wwwww   1/1     Running   0          2m
```

#### 8.3 Pod environment

Make sure that the pod has AWS SDK environment variables for IRSA:

```bash
POD_NAME=$(kubectl get pods -n ${K8S_NAMESPACE} \
  -l app.kubernetes.io/name=agentic-identity-broker \
  --output jsonpath='{.items[0].metadata.name}')

kubectl exec -n ${K8S_NAMESPACE} $POD_NAME -- env | grep AWS
```

Expected output:

```
AWS_ROLE_ARN=arn:aws:iam::ACCOUNT:role/AgenticIdentityBrokerEncryptionRole-prod
AWS_WEB_IDENTITY_TOKEN_FILE=/var/run/secrets/eks.amazonaws.com/serviceaccount/token
AWS_REGION=us-east-1
```

The EKS Pod Identity Webhook adds these environment variables.

#### 8.4 Application logs

Make sure that the application can access AWS resources:

```bash
kubectl logs -n ${K8S_NAMESPACE} $POD_NAME | grep -i kms
kubectl logs -n ${K8S_NAMESPACE} $POD_NAME | grep -i dynamodb
```

The logs must not contain AWS credential or permission errors.

#### 8.5 Run Helm tests

Run Helm tests to examine deployment health:

```bash
helm test broker -n ${K8S_NAMESPACE}
```

### Step 9: Basic health

Make sure that the broker runs and responds:

```bash
# Port-forward to broker service
kubectl port-forward -n ${K8S_NAMESPACE} svc/broker-agentic-identity-broker 8000:8000 &

# Test health endpoint
curl http://localhost:8000/health
```

For end-to-end OAuth2 and encryption tests, read the integration tests and API documentation.

### Step 10: Monitor CloudWatch metrics

Examine encryption infrastructure metrics:

```bash
# View CloudWatch dashboard
aws cloudwatch get-dashboard \
  --dashboard-name AgenticIdentityBroker-Encryption-prod

# Check KMS API call metrics
aws cloudwatch get-metric-statistics \
  --namespace AWS/KMS \
  --metric-name ApiCallCount \
  --dimensions Name=KeyId,Value=$(echo $KMS_KEY_ARN | awk -F'/' '{print $2}') \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Sum

# Check DynamoDB read/write metrics
aws cloudwatch get-metric-statistics \
  --namespace AWS/DynamoDB \
  --metric-name ConsumedReadCapacityUnits \
  --dimensions Name=TableName,Value=$DYNAMODB_TABLE_NAME \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Sum
```

## Deployment checklist

After deployment, make sure that these conditions are true:

- [ ] CDK stack deployed successfully
- [ ] IAM role created with federated trust policy
- [ ] Trust policy includes correct OIDC provider ARN
- [ ] Trust policy condition keys match namespace and service account
- [ ] ServiceAccount has `iam.amazonaws.com/role` annotation
- [ ] Pods have AWS_ROLE_ARN and AWS_WEB_IDENTITY_TOKEN_FILE env vars
- [ ] Application logs show successful AWS SDK initialization
- [ ] No credential or permission errors in logs
- [ ] Health endpoint responds successfully
- [ ] CloudWatch dashboard shows KMS and DynamoDB metrics
- [ ] KMS API calls visible in CloudWatch
- [ ] DynamoDB read/write operations recorded

## Troubleshooting

### Issue: Pod Cannot Assume IAM Role

**Symptoms**:
```
AccessDenied: User: sts:assumed-role/eks-node-role/i-xxxxx is not authorized to perform: sts:AssumeRoleWithWebIdentity
```

**Diagnosis**:

1. Examine the ServiceAccount annotation:
   ```bash
   kubectl get sa ${K8S_SERVICE_ACCOUNT} -n ${K8S_NAMESPACE} -o yaml
   ```

2. Examine the OIDC provider trust relationship:
   ```bash
   aws iam get-role --role-name $IAM_ROLE_NAME \
     --query 'Role.AssumeRolePolicyDocument'
   ```

**Solutions:**

- Make sure that the ServiceAccount name matches the trust policy condition.
- Make sure that the namespace matches the trust policy condition.
- Make sure that the OIDC provider ARN is correct.
- Examine the pod ServiceAccount:
  ```bash
  kubectl get pod $POD_NAME -n ${K8S_NAMESPACE} -o jsonpath='{.spec.serviceAccountName}'
  ```

### Issue: KMS Access Denied

**Symptoms**:
```
KMS.AccessDeniedException: User: arn:aws:sts::ACCOUNT:assumed-role/AgenticIdentityBrokerEncryptionRole-prod/xxxxx is not authorized to perform: kms:Decrypt
```

**Diagnosis:**

Examine IAM role permissions:
```bash
aws iam list-attached-role-policies --role-name $IAM_ROLE_NAME
aws iam list-role-policies --role-name $IAM_ROLE_NAME
```

**Solutions:**

- Deploy the CDK stack again. It manages permissions.
- Make sure that the KMS key ARN in Helm values matches the CDK output.
- Make sure that the KMS key policy allows the role.

### Issue: DynamoDB Access Denied

**Symptoms**:
```
DynamoDB.AccessDeniedException: User: arn:aws:sts::ACCOUNT:assumed-role/AgenticIdentityBrokerEncryptionRole-prod/xxxxx is not authorized to perform: dynamodb:GetItem
```

**Diagnosis:**

Examine DynamoDB table configuration:
```bash
aws dynamodb describe-table --table-name $DYNAMODB_TABLE_NAME
```

**Solutions:**

- Make sure that the DynamoDB table name in Helm values matches the CDK output.
- Make sure that the IAM role has DynamoDB permissions. The CDK stack manages these.
- Make sure that the table exists and is `ACTIVE`.

### Issue: OIDC Provider Not Found

**Symptoms**:
```
InvalidIdentityToken: No OpenIDConnect provider found
```

**Diagnosis:**

Make sure that the OIDC provider exists:
```bash
aws iam list-open-id-connect-providers
```

**Solutions:**

- Create the OIDC provider for the EKS cluster:
  ```bash
  eksctl utils associate-iam-oidc-provider --cluster my-cluster --approve
  ```
- Make sure that the OIDC provider ARN matches the CDK input.
- Make sure that the EKS cluster has an OIDC identity provider.

### Issue: Wrong Namespace or Service Account

**Symptoms**:
```
AssumeRoleWithWebIdentity: Condition not met (namespace mismatch)
```

**Diagnosis:**

Compare actual and expected values:
```bash
# Actual pod configuration
kubectl get pod $POD_NAME -n ${K8S_NAMESPACE} -o jsonpath='{.spec.serviceAccountName}'
kubectl get pod $POD_NAME -n ${K8S_NAMESPACE} -o jsonpath='{.metadata.namespace}'

# Expected subject used at deploy time (serviceAccountSubject)
# Format: system:serviceaccount:<namespace>:<service-account-name>
# Expected condition key: <oidc-provider-host>:sub (oidcSubjectKey)
```

**Solutions:**

- Make sure that Helm values match the CDK deployment parameters.
- Deploy the CDK stack again with the correct namespace and service account.
- Make sure that the pod is in the correct namespace.
- Make sure that the ServiceAccount name matches Helm values.

## Security Best Practices

### 1. Principle of Least Privilege

The CDK stack creates IAM roles with minimal permissions:
- KMS: Only `Decrypt` on specific key (no `Encrypt` needed - hierarchical keyring handles that)
- DynamoDB: Only `GetItem` and `PutItem` on branch key table
- No wildcard permissions
- No cross-account access

### 2. Namespace Isolation

Use separate namespaces for different environments:
```bash
# Development
kubectl create namespace identity-broker-dev

# Staging
kubectl create namespace identity-broker-staging

# Production
kubectl create namespace identity-broker-prod
```

Deploy separate CDK stacks with environment-specific IRSA roles.

### 3. Service Account Per Application

Do not share service accounts between applications. Each service must have:
- A dedicated ServiceAccount
- A dedicated IAM role
- A dedicated KMS key
- A dedicated DynamoDB table

### 4. Pod Security Standards

The Helm chart enforces restricted Pod Security Standards:
- Non-root user (UID 1000)
- Read-only root filesystem
- No privilege escalation
- All capabilities dropped
- Seccomp profile applied

### 5. Network Policies

Restrict network access to AWS services:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: broker-netpol
  namespace: identity-broker
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: agentic-identity-broker
  policyTypes:
    - Egress
  egress:
    # Allow AWS API endpoints
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443  # HTTPS for KMS, DynamoDB, STS
    # Allow DNS
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: UDP
          port: 53
```

### 6. Audit Logging

Enable AWS CloudTrail to audit IRSA operations:
```bash
aws cloudtrail lookup-events \
  --lookup-attributes AttributeKey=Username,AttributeValue=$IAM_ROLE_NAME \
  --max-results 50
```

Monitor for:
- `AssumeRoleWithWebIdentity` events
- KMS `Decrypt` operations
- DynamoDB `GetItem`/`PutItem` operations

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Deploy to EKS with IRSA

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          role-to-assume: arn:aws:iam::ACCOUNT:role/GitHubActionsDeployRole
          aws-region: us-east-1

      - name: Deploy CDK Stack
        run: |
          cd infra/cdk
          npm install -g aws-cdk
          npx cdk deploy \
            -c env=prod \
            -c oidcProviderArn="${{ secrets.EKS_OIDC_PROVIDER_ARN }}" \
            -c oidcSubjectKey="${{ secrets.EKS_OIDC_SUBJECT_KEY }}" \
            -c serviceAccountSubject=system:serviceaccount:identity-broker:broker-sa \
            --require-approval never

      - name: Extract Stack Outputs
        id: outputs
        run: |
          ROLE_ARN=$(aws cloudformation describe-stacks \
            --stack-name AgenticIdentityBrokerEncryptionVault-prod \
            --query 'Stacks[0].Outputs[?OutputKey==`EncryptionRoleARN`].OutputValue' \
            --output text)
          echo "role_arn=$ROLE_ARN" >> $GITHUB_OUTPUT

      - name: Update kubeconfig
        run: |
          aws eks update-kubeconfig --name my-cluster --region us-east-1

      - name: Deploy Helm Chart
        run: |
          helm upgrade --install broker ./charts/agentic-identity-broker \
            -n identity-broker \
            --set serviceAccount.irsa.enabled=true \
            --set serviceAccount.irsa.role=${{ steps.outputs.outputs.role_arn }}
```

## Cost Optimization

### KMS Key Rotation

Enable automatic key rotation (already configured in CDK):
- KMS automatically rotates backing keys annually
- Application transparently uses old and new keys
- No downtime or redeployment required

### DynamoDB On-Demand Pricing

The CDK stack uses on-demand billing for DynamoDB:
- No capacity planning required
- Pay only for requests
- Automatic scaling
- Suitable for variable workloads

### CloudWatch Dashboard

Monitor costs in CloudWatch dashboard:
- KMS request volume
- DynamoDB read/write capacity
- Set alarms for unexpected usage spikes

## Disaster Recovery

### KMS Key Recovery

KMS keys have a 30-day pending deletion window:

```bash
# Cancel pending deletion (within 30 days)
aws kms cancel-key-deletion --key-id $KMS_KEY_ARN
```

### DynamoDB Point-in-Time Recovery

The CDK stack enables PITR for DynamoDB:

```bash
# Restore to specific timestamp
aws dynamodb restore-table-to-point-in-time \
  --source-table-name $DYNAMODB_TABLE_NAME \
  --target-table-name ${DYNAMODB_TABLE_NAME}-restored \
  --restore-date-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S)
```

## Next Steps

- [Production deployment checklist](../operations/deployment-checklist.md)
- Plan KMS key management
- Define DynamoDB recovery procedures
- Set up monitoring and alerting
- Review security hardening requirements

## References

- [EKS IAM Roles for Service Accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
- [AWS CDK for Kubernetes](https://docs.aws.amazon.com/cdk/latest/guide/home.html)
- [KMS Envelope Encryption](https://docs.aws.amazon.com/kms/latest/developerguide/concepts.html#enveloping)
- [DynamoDB On-Demand Capacity](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/HowItWorks.ReadWriteCapacityMode.html)
