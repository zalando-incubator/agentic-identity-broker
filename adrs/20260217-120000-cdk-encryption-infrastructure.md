# ADR 20260217-120000: CDK Infrastructure for Encryption Resources

**Status**: Accepted
**Date**: 2026-02-17

---

## Context

[ADR 20260116-120200: Envelope Encryption Design](20260116-120200-envelope-encryption-design.md) established the three-layer key hierarchy (KEK → Branch Key → DEK) using AWS KMS Hierarchical Keyring with DynamoDB-backed branch key caching. The application code consumes these resources via `EncryptionConfig` (environment variables prefixed `IDENTITY_BROKER_ENCRYPTION_*`), but the infrastructure itself — the KMS key, DynamoDB table, and IAM role — must be provisioned consistently and reproducibly across environments.

Manual provisioning of encryption infrastructure introduces risk:

1. **Schema drift**: The AWS Encryption SDK KeyStore requires a DynamoDB table with a specific schema (`branch-key-id` (S) / `type` (S)). Manual creation is error-prone.
2. **Permission creep**: IAM policies must grant exactly the permissions the hierarchical keyring needs — no more, no less.
3. **Environment parity**: Test and production must differ only in safety controls (deletion protection, retention policies), not in resource topology.
4. **Auditability**: Infrastructure changes must be reviewable, versioned, and traceable.

---

## Decision

We will use **AWS CDK (Go SDK)** to define the encryption infrastructure as code in `infra/cdk/`, as a separate Go module within the monorepo.

### Why AWS CDK Go SDK

| Alternative         | Reason for rejection                                                                 |
|---------------------|--------------------------------------------------------------------------------------|
| Terraform           | Additional toolchain; team's primary language is Go, not HCL                         |
| CloudFormation YAML | Verbose, no type safety, no programmatic parameterization                            |
| Pulumi              | Viable, but CDK is the AWS-native IaC tool with first-party Go support               |
| AWS SDK (imperative)| Not declarative; no drift detection, no stack-level rollback                         |

CDK Go provides:
- **Type safety**: Compile-time checks for resource properties and cross-resource references
- **Assertions library**: Unit-testable infrastructure via `awscdk/assertions`
- **Same language as application**: Reduces context-switching; monorepo consistency
- **Parameterized stacks**: Single stack definition, environment-specific via CDK context

### Stack Design

A single `EncryptionStack` provisions:

1. **AWS KMS Symmetric CMK** — the Key Encryption Key (KEK) for the hierarchical keyring
   - Alias: `alias/agentic-identity-broker/{env}/token-vault-kek`
   - Automatic annual rotation enabled
   - Production: RETAIN removal policy, 30-day pending deletion window
   - Non-production: DESTROY removal policy, 7-day pending deletion window

2. **DynamoDB Table** — branch key cache for the KeyStore
   - Name: `AgenticIdentityBrokerBranchKeys-{env}`
   - Schema: `branch-key-id` (S, HASH) + `type` (S, RANGE) per AWS Encryption SDK spec
   - PAY_PER_REQUEST billing
   - Production: PITR enabled, deletion protection on
   - Non-production: no PITR, no deletion protection

3. **IAM Role** — least-privilege access for encryption operations
   - Name: `AgenticIdentityBrokerEncryptionRole-{env}`
   - Trust principal and subject condition set via `-c oidcProviderArn=<oidc-provider-arn> -c oidcSubjectKey=<oidc-provider-host>:sub -c serviceAccountSubject=<oidc-subject>` (required for all environments)
   - Permissions:
     - KMS: Encrypt, Decrypt, GenerateDataKey, ReEncrypt*, DescribeKey
     - KMS: CreateGrant (conditioned on `kms:GrantIsForAWSResource`)
     - DynamoDB: GetItem, PutItem, Query, UpdateItem, DeleteItem, DescribeTable

### Stack Outputs → Environment Variables

| Stack Output        | Environment Variable                                     |
|---------------------|----------------------------------------------------------|
| EncryptionKeyARN    | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN`             |
| BranchKeyTableName  | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME` |
| EncryptionRoleARN   | `IDENTITY_BROKER_ENCRYPTION_IAM_ROLE_ARN`                |

### Environment Parameterization

```bash
cdk deploy -c env=test \
  -c oidcProviderArn=<oidc-provider-arn> \
  -c oidcSubjectKey=<oidc-provider-host>:sub \
  -c serviceAccountSubject=system:serviceaccount:identity-broker:agentic-identity-broker
cdk deploy -c env=prod \
  -c oidcProviderArn=<oidc-provider-arn> \
  -c oidcSubjectKey=<oidc-provider-host>:sub \
  -c serviceAccountSubject=system:serviceaccount:identity-broker:agentic-identity-broker
```

### Tagging

All resources are tagged via `Tags.Of(stack).Add()`:

| Tag Key     | Value                      |
|-------------|----------------------------|
| application | agentic-identity-broker    |
| component   | encryption-vault           |
| environment | `{env}`                    |

---

## Consequences

### Positive

- **Single source of truth** for encryption infrastructure, versioned alongside application code
- **22 unit tests** validate KMS, DynamoDB, IAM, outputs, tags, and environment parameterization using CDK assertions
- **Justfile integration**: `just cdk-test`, `just cdk-synth`, `just cdk-deploy` for developer ergonomics
- **Separate Go module** (`infra/cdk/go.mod`) isolates CDK dependencies from the main application binary
- **Reproducible**: Any environment can be stood up with a single parameterized `cdk deploy` command

### Negative

- **Node.js runtime dependency**: CDK Go SDK uses jsii, which requires Node.js at synthesis time
- **CDK version coupling**: Stack must track CDK library updates for security patches
- **Learning curve**: Developers unfamiliar with CDK need to learn the constructs model

### Risks

- CDK Go SDK is less mature than the TypeScript SDK; some features may lag
- Tag propagation to KMS keys via stack-level APIs is unreliable in the Go SDK (mitigated by explicit `Tags.Of()` calls in the stack construct)

---

## References

- [ADR 20260116-120200: Envelope Encryption Design](20260116-120200-envelope-encryption-design.md)
- [ADR 20260116-120000: Encryption Context Optimization](20260116-120000-encryption-context-optimization.md)
- [AWS CDK Go SDK](https://pkg.go.dev/github.com/aws/aws-cdk-go/awscdk/v2)
- [AWS Encryption SDK – Hierarchical Keyring](https://docs.aws.amazon.com/encryption-sdk/latest/developer-guide/use-hierarchical-keyring.html)
