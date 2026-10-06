# ADR 040: Canonical light visual-reference gate

**Status**: Accepted
**Date**: 2026-10-06
**Decision reference**: The user's current request reduces visual references to one resolution and light mode instead of the six-way pixel gate.
**Feature**: [047-redesign-consent-console](../specs/047-redesign-consent-console/spec.md)
**Partially supersedes**: Only ADR 038's visual-reference matrix in [Single cutover and verification](038-design-system-rebuilt-on-shadcn-radix.md#single-cutover-and-verification).

## Context

ADR 038 required route references in both themes. Feature 047 extended Storybook pixel references to six theme/viewport projects.
The user considers that pixel gate excessive and authorizes one canonical reference configuration: light mode at 1280 × 720.

Constitution Principle XI requires light/dark rendering and accessibility checks. It also requires Storybook visual-regression testing, but does not prescribe both-theme pixel references.
This decision satisfies those separate requirements without a constitution amendment.

## Decision

### Canonical pixel references

All required PNG references use light mode at 1280 × 720. Dark and narrow configurations do not require pixel references.

Storybook captures and compares a reference only when all three conditions hold:

- The project is the desktop `storybook-light` project.
- The effective document theme is `light`.
- The actual viewport is 1280 × 720.

The comparison includes `document.body`, so portal overlays remain in the reference.
Intentional dark-theme or narrow-viewport story overrides still run their interactions and accessibility checks, without PNG capture or comparison.
Only eligible canonical cases receive reviewed Linux Chromium references in `web/.storybook/__screenshots__/`.

The route gate requires these eight files in `tests/e2e/screenshots/`:

| Route/context | Reference |
| --- | --- |
| `/agents` | `agents_light.png` |
| `/agents/:id` with authorization context | `agent_consent_light.png` |
| `/agents/:id` with console context | `agent_detail_light.png` |
| `/connections` | `connections_light.png` |
| `/approvals` | `approvals_light.png` |
| `/approvals/remembered` | `remembered_approvals_light.png` |
| `/approvals/:id` | `approval_review_light.png` |
| `/settings/appearance` | `settings_light.png` |

Required state references listed in `tests/e2e/screenshots/visual-gate.txt` use the same canonical configuration.
Other journey screenshots remain documentation, not additional required pixel references.
Legacy screenshots do not become required references through their presence in the directory.

### Preserved coverage and review

All six Storybook projects remain: light and dark at 375 × 812, 768 × 1024, and 1280 × 720.
They keep ordinary interaction tests and `parameters.a11y.test = 'error'`.
Dark-theme support, responsive layouts, keyboard paths, focus, reduced motion, and both-theme browser journeys remain required.

CI fails on missing or changed required canonical references. Existing pixel thresholds remain unchanged.
Automation generates review candidates, never approves images or updates reviewed references in CI.
The user's earlier approval of the larger reference set remains dated history, not a current matrix requirement.

### Capture determinism

Route fixtures use stable logical identifiers within fresh scenario storage. They do not share mutable entities or change authorization or server TTLs.
Story interactions use ordinary motion and an isolated pointer position.
Screenshot capture then normalizes the final state, uses reduced motion, and completes finite visual feedback.
The harness removes Storybook's portable-story animation pause before capture. Focus stories wait for real dismissal and focus restoration.
These capture rules do not change product animation behavior.

## Scope and consequences

This decision changes reference capture and comparison only. It introduces no theme selector, feature flag, or fallback reference path.
ADR 038's design, palette, typography, routing, APIs, authentication, authorization, and cutover decisions remain accepted.
The accepted 190-kB gzip decision-graph budget and five-second cold-consent target remain unchanged.

Reviewers examine one canonical pixel configuration instead of six. Automated interactions and accessibility checks still exercise the original theme/viewport matrix.
Dark and responsive regressions remain subjects of behavioral, accessibility, and browser checks, not mandatory PNG comparisons.

## References

- [ADR 038](038-design-system-rebuilt-on-shadcn-radix.md)
- [Constitution Principle XI](../.specify/memory/constitution.md#xi-design-system-compliance--consistency)
- [Feature specification](../specs/047-redesign-consent-console/spec.md)
- [Implementation plan](../specs/047-redesign-consent-console/plan.md#storybook-and-visual-gates)
- [UI validation contract](../specs/047-redesign-consent-console/contracts/ui-and-configuration.md#component-and-validation-contract)
- [Review quickstart](../specs/047-redesign-consent-console/quickstart.md#visual-snapshots)
- [Design principles](../web/src/design-system/docs/DESIGN_PRINCIPLES.md#accessibility-and-verification)
