# ADR 20260129-120000: Separate Migration Docker Image

**Status**: Accepted
**Date**: 2026-01-29
**Context**: Helm Chart Implementation (Feature 20260128-120000)

## Context

The Agentic Identity Broker requires database migrations to run before the application starts in Kubernetes deployments. Helm charts need to run migrations as a pre-install/pre-upgrade hook using a Kubernetes Job, separate from the main broker Deployment. This raises a critical security question: should we use a single Docker image with dual-purpose capability, or separate images for runtime and migrations?

### Security Concern

If we use a single image containing both the broker and `golang-migrate` binary:

**Attack Scenario**: An attacker who gains code execution within a running broker container (e.g., through an application vulnerability) could:
1. Access the `golang-migrate` binary at `/usr/local/bin/migrate`
2. Attempt to execute arbitrary SQL migrations
3. Even without migration credentials, the tool's presence expands the attack surface

While credentials can be properly separated at the Kubernetes level (different Secrets for migration vs broker), security best practice is to minimize capabilities in production runtime environments following the principle of defense-in-depth.

### Requirements

1. **Minimal Attack Surface**: Production broker image should contain only what's needed to run the application
2. **Separation of Concerns**: Migration tooling belongs in migration workloads, not runtime workloads
3. **Security by Design**: Follow least-privilege and defense-in-depth principles
4. **Maintainability**: Keep build and deployment processes manageable
5. **Version Consistency**: Ensure migrations match broker version

## Decision

**We split the Docker images: separate `agentic-identity-broker` and `agentic-identity-broker-migrate` images**

This approach prioritizes security through separation of capabilities over operational simplicity.

### Image Structure

**Image 1: `agentic-identity-broker` (Runtime)**
- Contains: Application binary, frontend assets
- Does NOT contain: golang-migrate binary, migration files
- Used by: Broker Deployment, regular Pods
- Security posture: Minimal, no SQL execution capabilities beyond application code

**Image 2: `agentic-identity-broker-migrate` (Migrations)**
- Contains: golang-migrate binary, migration files from `/migrations/`
- Does NOT contain: Application binary, frontend assets
- Used by: Kubernetes migration Job (Helm pre-install/pre-upgrade hook)
- Security posture: Restricted to migration Job with schema-level credentials

### Rationale

1. **Defense in Depth**: Even with proper credential separation, removing migration tooling from runtime reduces risk if broker is compromised

2. **Principle of Least Privilege**: Runtime containers should have minimal capabilities; SQL schema modification capability has no place in runtime

3. **Security Auditing**: Separate images make security boundaries explicit and easier to audit/scan

4. **Kubernetes Security Standards**: Aligns with Pod Security Standards by minimizing runtime capabilities

5. **Acceptable Trade-off**: Slightly increased CI/CD complexity is justified by improved security posture

## Consequences

### Positive

- **Enhanced Security**: Runtime broker cannot execute arbitrary SQL even if compromised
- **Clear Security Boundaries**: Separate images make capability separation explicit
- **Easier Security Scanning**: Runtime image has smaller attack surface, clearer audit scope
- **Compliance Friendly**: Easier to demonstrate least-privilege and defense-in-depth to auditors
- **Follows Best Practices**: Aligns with Kubernetes and container security guidelines

### Negative

- **Increased CI/CD Complexity**: Two images to build, push, scan, and version
- **Documentation Overhead**: Operators need to understand two-image structure
- **Storage Overhead**: Two images in registry (minimal impact, ~10MB difference)

### Neutral

- **Version Coupling**: Both images must stay synchronized (mitigated by shared appVersion)
- **Build Time**: Slightly longer CI pipeline (two builds vs one)

## Alternatives Considered

### Alternative 1: Single Image with Dual-Entrypoint

Use one image containing both the broker binary and golang-migrate, allowing command override:
- Default entrypoint: `/app/agentic-identity-broker` (broker mode)
- Override command: `/usr/local/bin/migrate` (migration mode)

**Pros**:
- Single image to build, version, and maintain
- Guaranteed version consistency between migrations and broker
- Simpler CI/CD pipeline
- Migration files always match broker version

**Cons**:
- Slightly larger broker image (~10MB for golang-migrate)
- Migration tool present in broker runtime (unused, but increases attack surface)
- Violates principle of least privilege

**Rejected because**: Security risk outweighs operational simplicity. Defense-in-depth principle requires minimizing runtime capabilities.

### Alternative 2: Remove Migrations from Image Entirely

Mount migrations from ConfigMap/EmptyDir/Git sync sidecar.

**Rejected because**: Breaks version coupling between migrations and application code. Increases deployment complexity significantly.

### Alternative 3: Multi-Stage Build with Shared Base

Build both images from shared layers to minimize duplication.

**Considered for future optimization**: Could reduce CI time and registry storage, but adds Dockerfile complexity. Can be implemented as optimization later without changing architecture.

## Security Analysis

### Attack Scenarios Mitigated

| Scenario | Single Image (Dual-Entrypoint) | Separate Images |
|----------|-------------------------------|-----------------|
| RCE in broker → Execute arbitrary SQL | Possible (migrate binary present) | Not possible (no migrate binary) |
| Container escape → SQL access | Limited by credentials | Limited by credentials |
| Compromised broker image | SQL capabilities present | No SQL capabilities |
| Supply chain attack on migrate tool | Affects all containers | Only affects migration Job |

### Remaining Risks

Both approaches still require:
- Proper Secret isolation (different credentials for migration vs broker)
- Network policies to restrict database access
- Regular security scanning of both images
- Pod Security Standards enforcement

## References

- OWASP Container Security: https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html
- Kubernetes Pod Security Standards: https://kubernetes.io/docs/concepts/security/pod-security-standards/
- CIS Docker Benchmark: https://www.cisecurity.org/benchmark/docker
- NIST SP 800-190: Application Container Security Guide

## Review History

- 2026-01-29: Initial draft (Feature 20260128-120000 implementation)
