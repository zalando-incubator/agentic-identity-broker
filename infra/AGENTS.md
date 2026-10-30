# Infrastructure — AWS CDK (`infra/`)

**Use retrieval-led reasoning. Before you change infrastructure, read `stack.go` and ADR 20260217-120000.**

## Overview

AWS CDK (Go SDK) provisions Token Vault encryption resources. `infra/cdk/` is a separate Go module.

**ADR**: [20260217-120000-cdk-encryption-infrastructure.md](../adrs/20260217-120000-cdk-encryption-infrastructure.md) is the binding decision for all infrastructure choices.

## Module Structure

`infra/cdk/` contains the CDK source and its tests.

## EncryptionStack Resources

Use the single `NewEncryptionStack()` function to provision:

| Resource | Type | Purpose |
|---|---|---|
| **KMS CMK** | `AWS::KMS::Key` | Symmetric KEK for hierarchical keyring. Annual rotation enabled. Alias: `alias/agentic-identity-broker/{env}/token-vault-kek` |
| DynamoDB Table | `AWS::DynamoDB::Table` | Branch-key cache with `branch-key-id` and `type` keys |
| **IAM Role** | `AWS::IAM::Role` | Least-privilege KMS + DynamoDB access via IRSA. Name: `AgenticIdentityBrokerEncryptionRole-{env}` |
| **CloudWatch Alarms** | `AWS::CloudWatch::Alarm` | KMS throttle + error detection |
| **CloudWatch Dashboard** | `AWS::CloudWatch::Dashboard` | KMS API ops + DynamoDB cache metrics |

### Environment Parameterization

Use `test`, `sandbox`, or `prod` as environment values. `production` is an input alias.
The CDK application changes it to `prod` before it names resources or chooses policies.

| Aspect | Production | Non-Production |
|---|---|---|
| KMS RemovalPolicy | RETAIN | DESTROY |
| KMS PendingDeletion | 30 days | 7 days |
| DynamoDB PITR | Enabled | Disabled |
| DynamoDB DeletionProtection | On | Off |
| IRSA parameters | Required | Required |

### Stack Outputs → Environment Variables

| Stack Output | Environment Variable |
|---|---|
| `EncryptionKeyARN` | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN` |
| `BranchKeyTableName` | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME` |
| `EncryptionRoleARN` | `IDENTITY_BROKER_ENCRYPTION_IAM_ROLE_ARN` |

### IRSA (IAM Roles for Service Accounts)

Use federated web identity trust through an IAM OIDC provider in all environments. Set these props:

- `ServiceAccountSubject` — Kubernetes service account subject claim (`system:serviceaccount:<namespace>:<sa-name>`)
- `OIDCProviderArn` — full OIDC provider ARN
- `OIDCSubjectKey` — IAM condition key for the subject claim (`<oidc-provider-host>:sub`)

## IAM Permissions (Least Privilege)

**KMS**: Encrypt, Decrypt, GenerateDataKey, GenerateDataKeyWithoutPlaintext, ReEncryptFrom, ReEncryptTo, DescribeKey, CreateGrant.

**DynamoDB**: GetItem, PutItem, Query, UpdateItem, DeleteItem, and DescribeTable. Apply permissions only to the branch-key table.

## Development Commands

```bash
just cdk-deps           # Install dependencies for the separate CDK module
just cdk-test           # Run CDK unit tests
just cdk-synth test \
  '-c serviceAccountSubject=...' \
  '-c oidcProviderArn=...' \
  '-c oidcSubjectKey=...' # Synthesize CloudFormation
```

All synth, diff, deploy, and destroy commands require `serviceAccountSubject`, `oidcProviderArn`, and `oidcSubjectKey` (ADR 20260703-120000).

Use these manual CDK commands:

```bash
cd infra/cdk
cdk synth -c env=test \
  -c serviceAccountSubject=system:serviceaccount:identity-broker:agentic-identity-broker \
  -c oidcProviderArn=arn:aws:iam::ACCOUNT:oidc-provider/... \
  -c oidcSubjectKey=oidc.eks.eu-central-1.amazonaws.com/id/EXAMPLE:sub
cdk diff -c env=test \
  -c serviceAccountSubject=system:serviceaccount:identity-broker:agentic-identity-broker \
  -c oidcProviderArn=arn:aws:iam::ACCOUNT:oidc-provider/... \
  -c oidcSubjectKey=oidc.eks.eu-central-1.amazonaws.com/id/EXAMPLE:sub
cdk deploy -c env=prod \
  -c serviceAccountSubject=system:serviceaccount:identity-broker:agentic-identity-broker \
  -c oidcProviderArn=arn:aws:iam::ACCOUNT:oidc-provider/... \
  -c oidcSubjectKey=oidc.eks.eu-central-1.amazonaws.com/id/EXAMPLE:sub
cdk destroy -c env=test \
  -c serviceAccountSubject=system:serviceaccount:identity-broker:agentic-identity-broker \
  -c oidcProviderArn=arn:aws:iam::ACCOUNT:oidc-provider/... \
  -c oidcSubjectKey=oidc.eks.eu-central-1.amazonaws.com/id/EXAMPLE:sub # Tear down a non-production stack
```

## Alignment Rules

- Make stack resources match the `internal/adapters/encryption/aws/` configuration and stack outputs.
- Make the DynamoDB schema match the AWS Encryption SDK KeyStore: `branch-key-id` (S) and `type` (S).
- Use the `application`, `component`, and `environment` tags for resources. Use `EncryptionStackProps.Tags` to add or override the first two.
- Keep `docs/operations/deployment-checklist.md`, `docs/deployment/kubernetes-irsa.md`, and ADR 20260217-120000 current.

## Testing

Use CDK `assertions.Template_FromStack()` and `createTestStack()` to examine synthesized CloudFormation. Test:

- KMS key properties (symmetric, rotation, removal policy, pending window).
- DynamoDB schema, billing, PITR, and deletion protection.
- IAM role trust policy (IRSA conditions) and permission statements.
- Stack outputs and export names.
- Tag propagation.
- Environment behavior for sandbox and production.
- Required IRSA configuration.
