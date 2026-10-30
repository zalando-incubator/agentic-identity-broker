# ADR 20260703-120000: Vendor-Neutral OIDC Trust

**Status**: Accepted
**Date**: 2026-07-03

---

## Context

The repository is being prepared for open-source use. The CDK stack that provisions encryption infrastructure previously depended on baked deployment placeholders for its IAM web-identity trust policy.

That approach is no longer appropriate for the public repository:

1. **Vendor coupling**: Open-source CDK source should not embed delivery-system-specific trust placeholders.
2. **Environment-specific trust**: The IAM federated principal ARN and the condition key for the OIDC subject claim vary by cluster and identity provider.
3. **Reviewability**: Trust inputs should be explicit at synth time so reviewers can see which OIDC provider and subject binding are being applied.
4. **Internal continuity**: The separate internal deployment repository still needs a way to preserve its placeholder-based CloudFormation flow without reintroducing those strings here.

---

## Decision

The CDK stack requires deployers to provide three explicit context parameters at synth or deploy time:

- `oidcProviderArn` — the IAM OIDC provider ARN used as the federated principal
- `oidcSubjectKey` — the IAM condition key for the OIDC subject claim, for example `<oidc-provider-host>:sub`
- `serviceAccountSubject` — the Kubernetes service-account subject value allowed to assume the role

The public repository will describe this trust relationship in vendor-neutral IAM OIDC and IRSA terms only. No baked CDP placeholders remain in the open-source CDK source.

Internal deployment automation may still pass its existing placeholder strings through these context parameters at synth time when it needs placeholder-preserving templates.

---

## Consequences

### Positive

- **Open-source neutrality**: The public repository no longer carries delivery-system-specific trust strings.
- **Explicit contract**: Deployers must provide the exact IAM OIDC trust inputs required for their environment.
- **Portable documentation**: CDK usage examples now reflect standard IAM OIDC federation concepts rather than one internal platform.
- **Clean separation**: Internal placeholder injection remains possible in the separate deployment repository without polluting this repository.

### Negative

- **Stricter deployment contract**: `cdk synth`, `cdk diff`, and `cdk deploy` now require all three trust parameters.
- **Coordinated rollout**: Any automation that synthesizes this stack must be updated to pass the new parameters.

### Risks

- Missing context values now fail synthesis instead of silently falling back to an unsafe or vendor-specific default.
- Incorrect `oidcSubjectKey` values can produce a trust policy that synthesizes successfully but rejects `AssumeRoleWithWebIdentity` at runtime.

---

## References

- [ADR 20260217-120000: CDK Infrastructure for Encryption Resources](20260217-120000-cdk-encryption-infrastructure.md)
- [Issue #401: Make repo internal-tooling / Zalando-CDP agnostic for open-sourcing](https://github.com/zalando-incubator/agentic-identity-broker/issues/401)
- [AWS IAM: Create a role for OpenID Connect federation](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-idp_oidc.html)
