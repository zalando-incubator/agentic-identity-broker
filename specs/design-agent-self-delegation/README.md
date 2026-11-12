# Worked cases: agents acting on their own behalf

Instance diagrams for the six worked cases of the design in
[issue 478](https://github.com/zalando-incubator/agentic-identity-broker/issues/478).

Read each drawing top to bottom: an agent has one agent grant, the grant names permission
sets, and each permission set is scoped at a service. The line down the side shows which of
the service's accounts the grant uses: solid when the grant names it, dashed when the service
has only one. Blue marks the records the design adds.

## Case 1: shared internal platform

![Two agents, two grants, one shared broker-signed account](case-1-shared-internal-platform.svg)

## Case 2: shared Google service account

![Two agents, two grants, one shared Google service account](case-2-shared-google-service-account.svg)

## Case 3: shared GitHub App installation

![Two agents, two grants, one shared GitHub App account](case-3-shared-github-app-installation.svg)

## Case 4: Linear, one account per level

![Linear: each grant is bound to the account for its level](case-4-linear-account-per-level.svg)

## Case 5: a grant beyond the ceiling

![A grant beyond the bound account ceiling is refused](case-5-grant-beyond-ceiling.svg)

## Case 6: a human behind the same agent

![A human subject: user grant and user session, no service account](case-6-human-subject.svg)
