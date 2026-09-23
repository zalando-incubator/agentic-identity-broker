---
applyTo: "infra/**"
---

# Infrastructure — AWS CDK

Full reference: `infra/AGENTS.md`

**ADR**: `adrs/010-cdk-encryption-infrastructure.md` — binding decision for all infrastructure choices.

## Module Structure

Separate Go module at `infra/cdk/` (Go 1.27.1, aws-cdk-go/awscdk/v2).

```
infra/cdk/
  cdk.json       CDK app config
  go.mod         Separate Go module
  main.go        Entrypoint — env parsing, validation, stack instantiation
  stack.go       EncryptionStack — all resources
  stack_test.go  CDK assertions unit tests (22+ tests)
```

## EncryptionStack Resources

| Resource | Type | Purpose |
|---|---|---|
| KMS CMK | `AWS::KMS::Key` | Symmetric KEK for hierarchical keyring. Annual rotation. |
| DynamoDB Table | `AWS::DynamoDB::Table` | Branch key cache. Schema: `branch-key-id` (S, HASH) + `type` (S, RANGE). |
| IAM Role | `AWS::IAM::Role` | Least-privilege KMS + DynamoDB access via IRSA. |
| CloudWatch | Alarms + Dashboard | KMS throttle/error detection, API ops metrics. |

### Environment Parameterization

Two environments: `test`, `prod`.

| Aspect | Production | Non-Production |
|---|---|---|
| KMS RemovalPolicy | RETAIN | DESTROY |
| DynamoDB PITR | Enabled | Disabled |
| DynamoDB DeletionProtection | On | Off |
| IRSA parameters | **Required** (panics if missing) | Optional |

### Stack Outputs → Environment Variables

| Output | Env Var |
|---|---|
| `EncryptionKeyARN` | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN` |
| `BranchKeyTableName` | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME` |
| `EncryptionRoleARN` | `IDENTITY_BROKER_ENCRYPTION_IAM_ROLE_ARN` |

## Alignment Rules

- Stack resources **must** match what `internal/adapters/encryption/aws/` expects.
- DynamoDB table schema **must** match AWS Encryption SDK KeyStore spec: `branch-key-id` (S) + `type` (S).
- All resources tagged: `Project=agentic-identity-broker`, `Component=encryption-vault`, `Environment={env}`, `ManagedBy=aws-cdk`.

## Testing

CDK `assertions.Template_FromStack()` for synthesized CloudFormation validation. `createTestStack()` helper for consistent setup. Test categories: KMS properties, DynamoDB schema, IAM trust policy, stack outputs, tags, env parameterization.

## Commands

```bash
just cdk-test     # Unit tests
just cdk-synth    # Synthesize CloudFormation
just cdk-deploy   # Deploy (default: test)
```
