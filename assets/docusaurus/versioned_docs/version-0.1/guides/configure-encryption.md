---
title: "Configure encryption at rest"
description: Configure the required encryption backend. Use a raw AES-256 key for development or an AWS KMS hierarchical keyring for production. Configure a separate JWE signing key for state and session tokens.
---

# Configure encryption at rest

The broker encrypts third-party OAuth2 access and refresh tokens, provider client secrets,
and signing-key private material at rest. Encryption is mandatory. The broker starts only
when exactly one encryption backend is configured. It does not fall back to plaintext. This
page explains each backend and how production pods access its keys.

For the key model and per-service context binding, see
[encryption at rest](../concepts/encryption.md).

## Choose a backend

Configure exactly one backend under `encryption`. Configuring both backends or neither
backend is a configuration error. The broker exits at startup.

| Backend | Key | Use for |
|---|---|---|
| `encryption.memory` | A raw base64 AES-256 key you supply | Development, testing, CI |
| `encryption.aws_kms` | An AWS KMS customer-managed key + a DynamoDB branch-key table | Production |

The memory backend keeps key material in the process environment. It has no key rotation,
hardware-backed key storage, or centralized audit. Use it only outside production.

## Development: a raw AES-256 key

The memory backend encrypts with a single 32-byte AES-256 key you provide,
base64-encoded.

### Generate a key

```bash
openssl rand -base64 32
```

### Configure it

Use an environment variable for the key. Do not write the key in the configuration file.
This prevents the key from entering version control:

```yaml
encryption:
  memory:
    # Base64-encoded 32-byte AES-256 key, injected from the environment.
    raw_key: "${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}"
```

The broker substitutes `${VAR}` values in YAML. It reads the value from the environment at
load time:

```bash
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(openssl rand -base64 32)"
```

:::warning
Do not commit a key or hardcode it in a configuration file. A process-environment reader
can access the memory-backend key. The memory backend does not rotate the key. Use it only
for development.
:::

## Production: AWS KMS hierarchical keyring

In production, the broker uses an AWS KMS customer-managed key as the root key encryption
key (KEK). It uses a DynamoDB table to cache intermediate branch keys. The KMS key is used
once for each service and TTL period. The broker does not call KMS for every token
operation. Each data key remains wrapped by KMS.

### AWS resources you need

- **A customer-managed KMS key (CMK)** — This root key wraps branch keys. AWS manages its
  key rotation.
- **A DynamoDB table** — This table stores derived branch keys. The cache prevents a KMS
  request for every encryption operation.

The project AWS CDK stack in `infra/cdk` can provision both resources. It creates the KMS
key, a DynamoDB table with the AWS Encryption SDK schema, and a least-privilege IAM role.
See [deployment checklist](../operations/deployment-checklist.md) for commands and resource
ARNs.

### Configure it

```yaml
encryption:
  aws_kms:
    # Customer-managed KMS key ARN.
    # Format: arn:aws:kms:<region>:<account-id>:key/<key-id>
    key_arn: "${IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN}"

    # DynamoDB table that caches branch keys.
    dynamodb_table_name: "IdentityBrokerEncryptionBranchKeys"

    # How long a cached branch key lives before regeneration (1m–24h).
    # Shorter = faster key rotation but more KMS calls.
    branch_key_ttl: "1h"

    # Optional: DynamoDB region, if it differs from the default SDK region.
    # dynamodb_region: "eu-central-1"
    # Optional: per-call timeout for AWS service calls.
    # dynamodb_timeout: "5s"
```

For production, set the region and structured JSON log format. See the full schema in
[configuration](../configuration.md).

### IAM permissions the broker needs

The broker identity must access the KMS key and the branch-key table:

- **KMS** — Encrypt, decrypt, and generate data keys with the customer-managed key. The
  keyring also describes the key and creates grants.
- **DynamoDB** — Read and write branch-key table items.

The CDK stack creates an IAM role for these actions and resources only. Limit each
hand-written policy to the key ARN and table. Do not grant account-wide KMS or DynamoDB
access.

### How pods reach the keys

On EKS, use **IAM Roles for Service Accounts (IRSA)** instead of static AWS credentials.
Annotate the broker service account with the IAM role for the KMS key and DynamoDB table.
The AWS SDK obtains and rotates temporary credentials. See the
[Kubernetes deployment guide](./deploy-on-kubernetes.md#grant-aws-access-with-irsa)
for operator steps.

## Configure the JWE signing key

The broker uses encrypted JWE tokens for short-lived state and consent sessions. These
tokens bind an OAuth2 callback to its initiating request. They prevent consent-screen
spoofing. They use a separate key, `third_party_oauth2.jwe_signing_key`. This is a
base64-encoded 32-byte key generated in the same way:

```yaml
third_party_oauth2:
  # Base64-encoded 32-byte key. Generate with: openssl rand -base64 32
  jwe_signing_key: "${IDENTITY_BROKER_THIRD_PARTY_OAUTH2_JWE_SIGNING_KEY}"
```

Provide this key in every environment. Inject it from a secret as you inject the encryption
key. Do not commit it.

## Examine startup output

At startup, the broker records the encryption backend that it initialized. It redacts key
material. The output shows the backend and its parameters, but not the key.

- **Success** — The broker starts and records the active memory or AWS KMS backend. For AWS
  KMS, the output includes the configured key ARN and table.
- **Failure closed** — A missing, duplicate, or malformed backend causes startup to stop. On
  Kubernetes, the pod enters `CrashLoopBackOff`. Examine the logs for the encryption error.

To make sure that KMS encryption works end to end, create a third-party OAuth2 session.
Then read the session. This stores and retrieves encrypted tokens through the complete
keyring. The [deployment checklist](../operations/deployment-checklist.md) includes this
smoke test.

## Related

- [Encryption at rest](../concepts/encryption.md) — the envelope model and
  per-service context binding.
- [Deploy on Kubernetes](./deploy-on-kubernetes.md) — wiring the KMS key,
  DynamoDB table, and IRSA role into the Helm release.
- [Configuration](../configuration.md) — the full `encryption` and
  `third_party_oauth2` schema.
- [Deployment checklist](../operations/deployment-checklist.md) — provisioning
  and verifying the encryption infrastructure.
