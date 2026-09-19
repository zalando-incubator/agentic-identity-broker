---
title: "Contributing"
description: How to contribute to the open-source Agentic Identity Broker project. This page covers issue reports, discussions, pull requests, and governance principles.
---

# Contributing

The Agentic Identity Broker is an open-source project. The repository contains its
license. Bug reports, discussions, and pull requests are welcome.

The repository defines the contribution process. This page gives an overview. Read these
source documents:

- **[CONTRIBUTING.md](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/CONTRIBUTING.md)**
  — the full contribution workflow.
- **[CODE_OF_CONDUCT.md](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/CODE_OF_CONDUCT.md)**
  — the community standards every participant agrees to.

## Ways to get involved

- **Report a bug.** Open a GitHub issue with clear steps that reproduce the problem.
- **Propose a feature or ask a question.** Start a GitHub discussion. This lets
  maintainers and contributors agree on direction before code is written.
- **Open a pull request.** Fork the repository. Work on a branch. Open a PR with a clear
  description, the reason for the change, and the test results. Reference related issues.

:::warning
Never open a public issue for a security vulnerability. Report it privately through the
process on the [Security posture](./security.md) page.
:::

## Governance principles

Contributions follow a specification-driven approach with a few standing principles:

- **API-first.** Public APIs are documented in their OpenAPI specification before they are
  implemented.
- **Security-first.** Security controls are enabled by default, never optional. Input is
  validated at system boundaries and secrets are never committed.
- **Tests required.** New functionality includes tests. The project checks and verification
  gate must pass before a pull request is merged.

## Code of conduct

The project follows the Contributor Covenant. Participants must use welcoming, inclusive
language and treat community members with respect. Report unacceptable behavior as the
[code of conduct](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/CODE_OF_CONDUCT.md)
describes.

## Before you start

If you want to understand the system before you contribute, run the local stack and
complete a delegation. See [Get started](../get-started/index.md).
