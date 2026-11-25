---
title: "Storage and encryption at rest"
description: Living specification of how a platform operator chooses where the broker keeps its data, brings the database schema up to date, and keeps stored provider tokens and client secrets unreadable without the key. Every example on this page is checked against tests.
---

# Storage and encryption at rest

The broker holds data that an attacker wants: Alice's GitHub tokens and the client secret that
the broker uses at GitHub. This page describes where that data lives, how the database schema is
brought up to date, and why a copy of the database is useless without the key.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

This page shows no HTTP exchanges. Its tests look at stored data, at ciphertext, and at the
database schema. Many of them need PostgreSQL or an AWS emulator in a container. Where no
container runtime is available, their badges show "not run".

## The cast

A **platform operator** deploys the broker. The operator makes two choices in the configuration
and runs one step before each upgrade:

- the storage backend: where agents, grants, services, and sessions are kept;
- the encryption backend: where the key comes from that protects stored secrets;
- the schema migrations, which run before the broker starts.

Alice (`alice@example.com`) has connected her account at the provider *GitHub*. Her
**UserSession** holds an access token and a refresh token. GitHub is registered as a
**ThirdpartyOAuth2Service** with a client secret. The administrator `admin@example.com` manages
agents such as the *Research Assistant* through the admin API.

## Two storage backends

`storage.backend` selects exactly one backend.

| Backend | Use | Data after a restart |
|---|---|---|
| `memory` (default) | Development and tests | Lost |
| `postgres` | Production, and any deployment with more than one replica | Kept |

The in-memory backend needs no setup. It is healthy as soon as it exists, and it creates, reads,
lists, updates, and deletes records until it is closed. `example:storage-and-encryption.backend.memory`

A value other than `memory` or `postgres` is a configuration error. `example:storage-and-encryption.backend.unknown`

The PostgreSQL backend needs a connection URL (illustration):

```yaml
storage:
  backend: postgres
  postgres:
    connection_url: ${IDENTITY_BROKER_STORAGE_POSTGRES_URL}
  timeouts:
    read: 5s
    write: 10s
```

Against a database whose schema is up to date, the PostgreSQL backend initializes without an
error. `example:storage-and-encryption.backend.postgres`

### The database must be there at startup

The broker initializes its storage when it starts. When PostgreSQL does not answer at the
configured address, initialization returns an error and the backend is not usable. `example:storage-and-encryption.startup.database-unreachable`

The broker does not fall back to the in-memory backend. A fallback would look healthy and lose
every session at the next restart.

The backend also looks for the table that records applied migrations. When that table is
missing, initialization stops with a message that tells the operator to run the migrations. No
test on this page proves this check yet.

## Schema migrations are a separate step

The broker does not change the database schema. Migrations are numbered SQL files with an "up"
and a "down" direction. The operator applies them with a separate migration image and a separate
database role before the new broker version starts. The page "Deployment" describes the Helm
Job that does this.

All migrations apply to an empty database. They also roll back to nothing and apply again, and
no step leaves the schema marked as dirty. `example:storage-and-encryption.migrations.lifecycle`

Each migration is one transaction: it applies completely or not at all. One migration is an
approved exception. It builds the index for session expiry without a transaction, so that it does
not block writes to a live session table. It also applies, rolls back, and applies again with
the session rows intact. `example:storage-and-encryption.migrations.session-expiry-index`

### A migration that finds bad data stops and changes nothing

A migration can add a rule that old data breaks. The migration that makes each agent's
**External ID** unique is the example. Older broker versions accepted two agents with the same
External ID, and agents with an empty External ID.

Suppose two agents both carry `urn:example:ai-registry:agent:7f3a9c`. The operator runs the
migrations. The migration stops with the error `agent external_id uniqueness migration blocked`
and names the UUIDs of both agents. No agent, grant, or client URI has changed. The old index is
still there and the unique index is not. The migration tool records the schema version as 47 and
dirty. The broker that was already running still serves both agents. `example:storage-and-encryption.migrations.external-id-blocked`

An agent with an empty External ID blocks the migration in the same way. The error reports the
duplicates and the empty values together. `example:storage-and-encryption.migrations.external-id-empty`

The check and the schema change are atomic at the database level. In every blocked case, the
rows of the agents and of everything that refers to them are identical before and after the
attempt. The error lists at most the first 100 duplicate groups and the first 100 agents with an
empty value. It gives a count of the rest. `example:storage-and-encryption.migrations.external-id-atomic`

The migration never picks a winner, merges agents, or deletes one. The operator decides:

1. Through the admin API of the running broker, `admin@example.com` gives one of the two agents
   a different External ID and removes the empty one. Each call addresses the agent by its UUID,
   so the agent and its grants stay as they are.
2. The operator confirms that the schema still matches the version before the migration, resets
   the recorded version to it, and runs the migrations again.

The second run succeeds and changes no row. From then on, an attempt to create another agent
with a taken External ID gets `409 Conflict`. `example:storage-and-encryption.migrations.external-id-remediated`

The migration can be rolled back and applied again. Agents, grants, and client URIs are the same
after each step. `example:storage-and-encryption.migrations.external-id-rollback`

The runbook `docs/operations/agent-external-id-uniqueness.md` gives the inventory queries and
the full procedure.

## Encryption at rest

### What is encrypted

The broker encrypts these values before it stores them:

- the access token and the refresh token of a **UserSession**;
- the client secret of a **ThirdpartyOAuth2Service**.

Encryption happens in the broker, not in the database. Both storage backends receive ciphertext
only. A token that goes through encryption comes out as different bytes. `example:storage-and-encryption.encryption.ciphertext`

With the matching key and context, decryption returns the original token. `example:storage-and-encryption.encryption.round-trip`

For a service with a client secret, the database column holds ciphertext, and the record that the
repository returns carries the secret in its encrypted state. `example:storage-and-encryption.encryption.client-secret-stored`

The secret stays encrypted while the broker only looks a service up. The broker decrypts it when
it reads the service for use. `example:storage-and-encryption.encryption.client-secret-round-trip`

The page "Broker credentials at a third-party service" describes credentials that live in
mounted files and are not stored at all. The page "Token issuance and refresh" describes the
broker's signing keys, which use the same encryption.

### One key per value, bound to its owner

The broker uses envelope encryption. Each encryption operation gets a new data key. The data key
encrypts the value. A key-encryption key then wraps the data key, and the wrapped data key is
stored with the ciphertext.

Because every operation uses a new data key, the same token encrypted twice for the same service
gives two different ciphertexts. Both decrypt to the same token. `example:storage-and-encryption.encryption.key-per-value`

Each ciphertext is bound to an **EncryptionContext**. For tokens and client secrets, the context
names the service that the value belongs to: `{"service_id": "…"}`. The context is not secret.
It is authenticated: decryption must present the same context.

A token that was encrypted for the context `{"service_id": "oauth2"}` does not decrypt with
`{"service_id": "invalid"}`. Decryption returns an error that names the context, and it returns
no bytes. `example:storage-and-encryption.encryption.context-mismatch`

This stops an attacker who can write to the database from moving a ciphertext to another service.
A ciphertext for the service `oauth2` does not decrypt as a value of the service `github`. The
rightful service still decrypts it afterwards. `example:storage-and-encryption.encryption.cross-service`

### Where the key-encryption key comes from

The operator configures exactly one encryption backend. A configuration without one is
rejected. `example:storage-and-encryption.encryption.backend-required` A configuration with
both is rejected too. `example:storage-and-encryption.encryption.one-backend` The broker does
not start with a rejected configuration, so it never runs without a usable key.

**Production: AWS KMS.** The key-encryption key stays in AWS KMS (illustration):

```yaml
encryption:
  aws_kms:
    key_arn: ${IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN}
    dynamodb_table_name: IdentityBrokerEncryptionBranchKeys
    branch_key_ttl: 1h
```

The broker does not call KMS for every token. Between the KMS key and the data keys sits a
**BranchKey**. A branch key is derived for one service, protected by the KMS key, and kept in a
DynamoDB table. The broker caches it for `branch_key_ttl`. The chain is: KMS key, then branch
key, then data key.

The context rule holds with this backend as well. A ciphertext for one service does not decrypt
with the context of another service. `example:storage-and-encryption.encryption.kms-context`

**Development and tests: a key in the configuration.** The operator supplies the key itself
(illustration):

```yaml
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
```

The value is a base64-encoded AES-256 key: exactly 32 bytes. A 16-byte key is rejected. `example:storage-and-encryption.encryption.memory-key-length`

The key must stay the same across restarts. A broker that starts again with the same key
decrypts what the earlier process stored. `example:storage-and-encryption.encryption.restart-same-key`
A key that is lost makes every stored token and secret unreadable.

When the broker rejects a key, for example because it is empty, too short, or not base64, the
error message does not contain the key. `example:storage-and-encryption.encryption.key-not-in-errors`

### No plaintext fallback

The broker fails closed in both directions.

- On write: when encryption fails while an administrator registers a service, the call fails and
  nothing is stored. The broker does not store the plaintext secret instead. `example:storage-and-encryption.encryption.no-plaintext-fallback`
- On read: when decryption fails, the caller gets an error and no value, as the context examples
  above show.

## Not yet on this page

- The startup check for a database without applied migrations has no test, so the page states
  it without a badge. The same holds for the full startup of the broker against an unreachable
  database: the test covers the storage backend, not the process.
- The session token examples call the encryption adapter directly. No test on this page reads a
  stored **UserSession** from PostgreSQL and checks that its token columns are ciphertext.
- AWS KMS behavior beyond the context rule: branch-key creation and rotation, key rotation with
  older ciphertexts, a KMS outage at startup or at run time, and the IAM setup.
- Post-quantum algorithms, connection pooling, storage timeouts, and the other migrations that
  refuse legacy data.
- The Helm migration Job and the migration image: see the page "Deployment".

## Sources

The behavior on this page was decided in `specs/004-persistence-layer/spec.md`,
`specs/012-aws-encryption-vault/spec.md`, and User Story 3 of
`specs/055-agent-external-id-upsert/spec.md`, and in `adrs/004-storage-layer-architecture.md`,
`adrs/008-encryption-context-optimization.md`, `adrs/009-envelope-encryption-design.md`,
`adrs/009-separate-migration-docker-image.md`, `adrs/012-encryption-layer-separation.md`,
`adrs/039-concurrent-session-expiry-index.md`, and
`adrs/047-unique-agent-external-id-reconcile.md`. The configuration contract is
`internal/ports/config.go`, described in `docs/configuration.md`.

The tests are in `tests/e2e/encryption_vault_raw_test.go`,
`tests/e2e/agent_external_id_upsert_test.go`, `tests/integration/storage/`,
`tests/integration/migrations/`, `tests/integration/infra/encryption_vault_keyring_test.go`,
`internal/config/encryption_test.go`, `internal/config/storage_test.go`,
`internal/adapters/encryption/aws/adapter_test.go`, and
`internal/domain/thirdparty/service_test.go`. The tests under `tests/integration/storage/infra/`,
`tests/integration/migrations/`, and `tests/integration/infra/` are compiled only with the build
tag `integration`.
