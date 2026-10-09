# Plan

Use existing Fosite handlers, consent verification, refresh repositories, and builder wiring. Keep main's constitution unchanged.

1. Write failing provider, lifecycle, storage, and production-builder acceptance tests. Use injected clocks and database lock barriers, not sleeps.
2. Add nullable grant identity and migration 036. Old binaries retain their explicit-column insert behavior.
3. Verify consent before token rotation. Carry the selected grant ID through the Fosite session, not a shared cache.
4. Add targeted revocation methods to memory and PostgreSQL. Keep the agent-before-refresh lock order inside the PostgreSQL repository.
5. Wire grant deletion, expired renewal, and credential deletion to revocation. Deletes remain separate because main's lifecycle repositories use the database directly.
6. Update existing architecture, OpenAPI response descriptions, and OAuth2 documentation. Record all three compatibility limits.
7. Run build, static checks, race unit tests, PostgreSQL integration, and backend E2E. Keep additions within 500 production Go/SQL lines and 2,000 total lines.

The transaction covers PostgreSQL refresh rotation only. Grant and credential deletion cannot roll back after a revocation error. There are no retries or new transaction interfaces.
