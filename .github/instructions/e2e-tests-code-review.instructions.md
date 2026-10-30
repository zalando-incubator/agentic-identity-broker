---
applyTo: "tests/e2e/**"
excludeAgent: "coding-agent"
---

# E2E Test Code Review Guidelines

These guidelines apply specifically to end-to-end tests in `tests/e2e/`. They enforce **Constitution Principle XIII** (End-to-End Acceptance Testing & Spec Traceability). For comprehensive patterns, see [tests/e2e/README.md](../../tests/e2e/README.md).

---

## Critical Requirements

### 1. Spec Traceability (MANDATORY)
**Every `It()` block MUST link to a specific acceptance scenario from `specs/[YYYYMMDD-HHMMSS-feature]/spec.md`.**

✅ Required:
```go
// Scenario 1.3 from specs/20251222-120100-oauth2-auth-server/spec.md (User Story 1)
It("should redirect to consent UI when no grant exists", func() {
```

❌ Flag for rejection:
```go
It("should redirect to consent", func() {  // Missing spec reference
```

**Action**: Request adding comment with format `// Scenario X.Y from specs/[feature]/spec.md`.

---

### 2. Test Structure: Use BeforeEach, Not Setup in It()

**Tests MUST use hierarchical BeforeEach blocks for shared setup, NOT heavy setup inside It() blocks.**

✅ Good:
```go
Describe("OAuth2 Authorization", func() {
    var (
        server      *bootstrap.TestServer
        testStorage *storageadapter.Adapter
        agent       *storage.Agent
    )

    BeforeEach(func() {
        // Suite-level: runs for ALL tests
        testStorage, _ = storageFactory.NewTestStorage()
        server, _ = bootstrap.NewTestServer(appInstance, logger)
    })

    Describe("when valid request arrives", func() {
        BeforeEach(func() {
            // Scenario-level: agent for tests in this Describe
            agent = fixtures.ValidAgent()
            testStorage.Agents().Create(ctx, agent)
        })

        It("should look up agent by client_id", func() {
            // Clean test - just action and assertions
            resp, _ := server.AuthenticatedGET(...)
            Expect(resp.StatusCode).To(Equal(http.StatusFound))
        })
    })
})
```

❌ Flag for rejection:
```go
It("should look up agent by client_id", func() {
    // Heavy setup in It() block
    config := fixtures.DefaultOAuth2Config()
    storage, _ := storageFactory.NewTestStorage()
    server, _ := bootstrap.NewTestServer(...)
    agent := fixtures.ValidAgent()
    testStorage.Agents().Create(ctx, agent)
    // ... 10 more lines of setup

    // Finally the test
    resp, _ := server.AuthenticatedGET(...)
})
```

**Action**: Request refactoring to BeforeEach blocks. **If It() block > 15 lines, likely contains setup code that should move to BeforeEach.**

---

### 3. Context for Variants, Not Duplicate Setup

**Test variants (e.g., different grant states) MUST use Context blocks, NOT duplicate setup in multiple It() blocks.**

✅ Good:
```go
Describe("when processing requests", func() {
    BeforeEach(func() {
        // Shared: all variants need agent
        agent = fixtures.ValidAgent()
        testStorage.Agents().Create(ctx, agent)
    })

    Context("and no grant exists", func() {
        It("should redirect to consent", func() { /* test */ })
    })

    Context("and active grant exists", func() {
        BeforeEach(func() {
            grant := fixtures.ActiveGrant(principal, agent.ID)
            testStorage.UserGrants().Create(ctx, grant)
        })
        It("should proxy to upstream", func() { /* test */ })
    })

    Context("and expired grant exists", func() {
        BeforeEach(func() {
            grant := fixtures.ExpiredGrant(principal, agent.ID)
            testStorage.UserGrants().Create(ctx, grant)
        })
        It("should redirect to consent", func() { /* test */ })
    })
})
```

❌ Flag for rejection:
```go
It("should handle no grant", func() {
    agent := fixtures.ValidAgent()  // Duplicate setup!
    testStorage.Agents().Create(ctx, agent)
    // test with no grant
})

It("should handle active grant", func() {
    agent := fixtures.ValidAgent()  // Same setup repeated!
    testStorage.Agents().Create(ctx, agent)
    grant := fixtures.ActiveGrant(...)
    // test with active grant
})
```

**Action**: Request Context blocks with shared BeforeEach setup.

---

### 4. Naming Conventions

| Element | Required Pattern | Example |
|---------|------------------|---------|
| Top-level Describe | Feature/endpoint name | `Describe("OAuth2 Authorization Endpoint", ...)` |
| Nested Describe | "when [condition]" | `Describe("when valid request arrives", ...)` |
| Context | "and [condition]" | `Context("and no grant exists", ...)` |
| It | "should [behavior]" | `It("should redirect to consent UI", ...)` |

**Typical length**: 5-12 words for It() descriptions.

❌ Flag for rejection:
- Vague: `It("works", ...)` or `It("test", ...)`
- Missing "should": `It("redirect to consent", ...)`
- Too long: 30+ word descriptions

**Action**: Request specific "should [behavior]" naming (5-12 words).

---

### 5. One Behavior Per It()

**Each It() MUST test ONE behavior, NOT multiple unrelated scenarios.**

❌ Flag for rejection:
```go
It("should handle edge cases", func() {
    // Test 1: Missing client_id
    resp1, _ := server.GET("/oauth2/authorize?redirect_uri=...")
    Expect(resp1.StatusCode).To(Equal(400))

    // Test 2: Missing redirect_uri
    resp2, _ := server.GET("/oauth2/authorize?client_id=test")
    Expect(resp2.StatusCode).To(Equal(400))

    // Test 3: Missing response_type
    resp3, _ := server.GET("/oauth2/authorize?client_id=test&redirect_uri=...")
    Expect(resp3.StatusCode).To(Equal(400))
})
```

**Action**: Request splitting into 3 separate It() blocks.

---

### 6. Use Fixtures, Not Hardcoded Data

**Tests MUST use fixtures from `tests/e2e/fixtures/`, NOT hardcoded structs.**

✅ Good:
```go
agent := fixtures.ValidAgent()
principal := fixtures.DefaultPrincipal().String()
grant := fixtures.ActiveGrant(principal, agent.ID)
```

❌ Flag for rejection:
```go
agent := &storage.Agent{
    ID:       "test-agent-id",
    ClientID: "test-client-123",
    Name:     "Test Agent",
    // ... 10 more fields
}
principal := "user@example.com"
```

**Action**: Request using fixture functions. Reference [tests/e2e/fixtures/README.md](../../tests/e2e/fixtures/README.md).

---

### 7. Given/When/Then Structure

**Tests MUST follow clear Given (setup) / When (action) / Then (assertions) structure.**

✅ Good:
```go
It("should return access token when code is valid", func() {
    // Given: (preconditions in BeforeEach - agent & grant created)

    // When: Exchange authorization code
    formData := url.Values{"grant_type": []string{"authorization_code"}, ...}
    resp, err := server.DirectRequest("POST", "/oauth2/token", principal, headers, formData)
    Expect(err).ToNot(HaveOccurred())
    defer resp.Body.Close()

    // Then: Verify token response
    Expect(resp.StatusCode).To(Equal(http.StatusOK))
    body, _ := io.ReadAll(resp.Body)
    Expect(body).To(ContainSubstring("access_token"))
})
```

❌ Flag for rejection (mixed setup/action/assertions):
```go
It("test", func() {
    resp, _ := server.GET(path)
    agent := fixtures.ValidAgent()
    Expect(resp.StatusCode).To(Equal(200))
    testStorage.Agents().Create(ctx, agent)
    body, _ := io.ReadAll(resp.Body)
})
```

**Action**: Request restructuring with clear Given/When/Then sections (comments optional but encouraged).

---

### 8. Shared Variables at Describe Level

**Shared variables MUST be declared at Describe level, initialized in BeforeEach.**

✅ Good:
```go
Describe("Feature", func() {
    var (
        server      *bootstrap.TestServer
        agent       *storage.Agent
    )

    BeforeEach(func() {
        agent = fixtures.ValidAgent()
        testStorage.Agents().Create(ctx, agent)
    })

    It("test 1", func() {
        // Uses agent variable
    })

    It("test 2", func() {
        // Also uses agent variable
    })
})
```

❌ Flag for rejection:
```go
It("test 1", func() {
    agent := fixtures.ValidAgent()  // Local variable
})

It("test 2", func() {
    agent := fixtures.ValidAgent()  // Recreated!
})
```

**Action**: Request moving shared variables to Describe-level var declaration.

---

### 9. Resource Cleanup in AfterEach

**All resources MUST be closed in AfterEach to prevent leaks.**

✅ Good:
```go
AfterEach(func() {
    if server != nil {
        server.Close()
    }
    if mockUpstream != nil {
        mockUpstream.Close()
    }
    if testStorage != nil {
        storageFactory.CloseStorage(testStorage)
    }
})
```

❌ Flag for rejection:
```go
// No AfterEach - resource leak!
```

**Action**: Request AfterEach with cleanup for all resources created in BeforeEach.

---

### 10. Test Independence

**Tests MUST be independent - fresh storage and fixtures for EACH test.**

✅ Good:
```go
BeforeEach(func() {
    // Fresh storage for EACH test
    testStorage, _ = storageFactory.NewTestStorage()
    agent = fixtures.ValidAgent()  // Fresh agent each time
})
```

❌ Flag for rejection:
```go
var sharedAgent *storage.Agent  // Shared across tests!

BeforeEach(func() {
    if sharedAgent == nil {  // Only creates once
        sharedAgent = fixtures.ValidAgent()
    }
})
```

**Action**: Request creating fresh fixtures for each test (no shared state).

---

## Quick Anti-Pattern Detection

### 🚨 Flag These Patterns for Rejection:

1. **Heavy It() blocks**: >15 lines in It() block → likely has setup code that should be in BeforeEach
2. **Flat structure**: All tests at same level with duplicate setup → needs Describe/Context organization
3. **Multiple behaviors**: Single It() testing 3+ unrelated scenarios → split into separate It() blocks
4. **Vague names**: "works", "test", or descriptions >20 words → use "should [behavior]" (5-12 words)
5. **Hardcoded data**: Struct literals instead of fixtures → use `fixtures.*()` functions
6. **Missing spec link**: No comment referencing spec scenario → add `// Scenario X.Y from specs/...`
7. **Mixed structure**: Setup mixed with actions/assertions → enforce Given/When/Then
8. **Local variables**: Recreating same variable in multiple It() → declare at Describe level
9. **No cleanup**: Missing AfterEach or missing Close() calls → add resource cleanup
10. **Shared state**: Variables shared across tests → create fresh fixtures per test

---

## Review Checklist

Use this for E2E test PR reviews:

- [ ] Each It() has spec reference (// Scenario X.Y from specs/...)
- [ ] BeforeEach used for setup (not heavy setup in It() blocks)
- [ ] Context blocks used for test variants (not duplicate setup)
- [ ] Naming follows "when/and/should" pattern
- [ ] One behavior per It() block
- [ ] Fixtures used (not hardcoded structs)
- [ ] Given/When/Then structure clear
- [ ] Shared variables declared at Describe level
- [ ] AfterEach cleanup present
- [ ] Tests independent (fresh storage/fixtures per test)

---

## Constitution Compliance

These guidelines enforce [Constitution Principle XIII](../../.specify/memory/constitution.md):
- Each It() maps 1:1 to spec scenario (traceability)
- Ginkgo/Gomega BDD framework
- Hierarchical Describe/Context/It structure
- Complete system integration testing
- Production bootstrap code (app.Builder)
- Fixture-based test data
- Given/When/Then structure
- Test independence and isolation

For comprehensive guidance: [tests/e2e/README.md](../../tests/e2e/README.md) (2000+ lines with examples, anti-patterns, patterns).

---

**Version**: 1.0.0 (aligned with Constitution v1.7.0)
**Last Updated**: 2026-01-06
