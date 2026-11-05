# Infrastructure — AWS CDK (`infra/`)

All paths in this file are relative to the repository root.

Before you change infrastructure, read `infra/cdk/stack.go` and the relevant sources in this table.

## Overview

AWS CDK (Go SDK) provisions Token Vault encryption resources. `infra/cdk/` is a separate Go module.

| Source | Purpose |
|---|---|
| `ARCHITECTURE.md`, `.specify/memory/constitution.md` | Infrastructure boundary, security requirements, and workload configuration rules |
| `adrs/010-cdk-encryption-infrastructure.md` | Accepted CDK resource design |
| `adrs/030-vendor-neutral-oidc-trust.md` | Accepted trust inputs for every environment |
| `adrs/009-envelope-encryption-design.md` | Accepted envelope encryption design |
| `specs/012-aws-encryption-vault/spec.md`, `specs/012-aws-encryption-vault/plan.md`, `specs/012-aws-encryption-vault/tasks.md` | Encryption requirements and implementation history |
| `internal/ports/config.go`, `internal/adapters/encryption/aws/config.go` | Current application configuration and AWS credential selection |
| `docs/configuration.md` | Current configuration reference |
| `docs/operations/deployment-checklist.md`, `docs/deployment/kubernetes-irsa.md` | Deployment procedures and trust inputs |
| `charts/agentic-identity-broker/templates/serviceaccount.yaml` | Helm service-account annotation |

The feature documents mark their original single-field encryption configuration as superseded.
Use the current `encryption.aws_kms` contract, not the historical `encryption.key` proposal.

## Module Structure

| Path | Purpose |
|---|---|
| `infra/cdk/main.go` | Context inputs, environment validation, AWS account and region, and stack creation |
| `infra/cdk/stack.go` | Resources, trust policy, permissions, outputs, and tags |
| `infra/cdk/stack_test.go` | Synthesized CloudFormation assertions and test helpers |
| `infra/cdk/go.mod` | Separate Go toolchain and dependency versions |
| `infra/cdk/cdk.json` | CDK application command and default context |
| `justfile` | Supported repository commands |

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
| KMS and DynamoDB RemovalPolicy | RETAIN | DESTROY |
| KMS PendingDeletion | 30 days | 7 days |
| DynamoDB PITR | Enabled | Disabled |
| DynamoDB DeletionProtection | On | Off |
| IRSA parameters | Required | Required |

### Stack Outputs and Application Configuration

| Stack Output | Consumer |
|---|---|
| `EncryptionKeyARN` | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN` |
| `BranchKeyTableName` | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME` |
| `EncryptionRoleARN` | Helm `serviceAccount.irsa.role` for workload identity |

`IDENTITY_BROKER_ENCRYPTION_IAM_ROLE_ARN` is not a field in the current application configuration.
ADR 010 still lists this historical mapping.
The separate `encryption.aws_kms.assume_role_arn` field uses STS `AssumeRole`, not the stack's web-identity trust.
Do not substitute that field for IRSA.
Before deployment, compare the chart's service-account annotation with the target cluster's identity integration.

### IRSA (IAM Roles for Service Accounts)

Use federated web identity trust through an IAM OIDC provider in all environments. Set these props:

- `ServiceAccountSubject` — Kubernetes service account subject claim (`system:serviceaccount:<namespace>:<sa-name>`)
- `OIDCProviderArn` — full OIDC provider ARN
- `OIDCSubjectKey` — IAM condition key for the subject claim (`<oidc-provider-host>:sub`)

Keep `sts:AssumeRoleWithWebIdentity`, the federated provider, and the exact subject condition in the synthesized trust policy.
Do not retain the temporary account-root principal from the role constructor.
Do not embed deployment-system placeholders in the source.

## IAM Permissions (Least Privilege)

**KMS**: Encrypt, Decrypt, GenerateDataKey, GenerateDataKeyWithoutPlaintext, ReEncryptFrom, ReEncryptTo, DescribeKey, CreateGrant.
Scope these permissions to the stack's KMS key.
Keep CreateGrant in a separate statement with `kms:GrantIsForAWSResource=true`.

**DynamoDB**: GetItem, PutItem, Query, UpdateItem, DeleteItem, and DescribeTable. Apply permissions only to the branch-key table.

## Development Commands

Run repository recipes from the repository root.
`just cdk-deps` runs `go mod tidy` and `go mod download` in the separate module.
The Go CDK application also requires Node.js for jsii.

```bash
just cdk-deps           # Install dependencies for the separate CDK module
just cdk-test           # Run CDK unit tests
just cdk-synth test \
  '-c serviceAccountSubject=...' \
  '-c oidcProviderArn=...' \
  '-c oidcSubjectKey=...' # Synthesize CloudFormation
```

All synth, diff, deploy, and destroy commands require `serviceAccountSubject`, `oidcProviderArn`, and `oidcSubjectKey`.
Read `adrs/030-vendor-neutral-oidc-trust.md` for this contract.

Use manual CDK commands for diff, deploy, and destroy. The `justfile` has no recipes for these operations.
Replace the example trust inputs with the target provider and service account.
Review the diff before deployment.
CAUTION: Delete only the intended stack. Non-production removal policies delete the branch-key table and schedule KMS key deletion.

Start these manual commands from the repository root with the CDK CLI available:

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
- Use the `application`, `component`, and `environment` tags for taggable resources. Use `EncryptionStackProps.Tags` for overrides or additions.
- Keep the `environment` tag equal to the normalized stack environment.
- Preserve the template metadata header (`StackName` and `Tags`) that deployment tooling expects.
- Keep `docs/operations/deployment-checklist.md`, `docs/deployment/kubernetes-irsa.md`, and ADR 010 current.

## Testing

Use `assertions.Template_FromStack()` and `createTestStack()` in `infra/cdk/stack_test.go` to examine synthesized CloudFormation.
Cover these areas:

- KMS key properties (symmetric, rotation, removal policy, pending window).
- DynamoDB schema, billing, PITR, and deletion protection.
- IAM role trust policy (IRSA conditions) and permission statements.
- Stack outputs, export names, and template metadata.
- Tag propagation.
- Environment behavior for test, sandbox, prod, and the production alias.
- Required IRSA configuration and rejection of missing trust inputs.
