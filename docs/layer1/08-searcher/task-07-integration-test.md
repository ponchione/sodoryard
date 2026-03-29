# Task 07: Integration Test

**Epic:** 08 — Searcher
**Status:** ⬚ Not started
**Dependencies:** Task 05, L1-E04 (embedding client), L1-E05 (LanceDB store)

---

## Description

Write an integration test that exercises the full searcher stack with real (or realistic) dependencies: a LanceDB store seeded with test chunks that have known relationships, and either a real embedding client (guarded by build tag) or a fake embedder that produces consistent vectors. The test verifies that multi-query search, deduplication, re-ranking, and hop expansion produce the expected results against actual stored data rather than mocks.

## Acceptance Criteria

### Test Data Setup

- [ ] Test file at `internal/rag/searcher/integration_test.go`, guarded by build tag `//go:build integration`
- [ ] Seed a temporary LanceDB store with at least 10 chunks representing a realistic mini-codebase:
  - Chunk "AuthMiddleware" (func, `internal/auth/middleware.go`): Calls=["ValidateToken", "GetUserClaims"], CalledBy=["RegisterRoutes"]
  - Chunk "ValidateToken" (func, `internal/auth/service.go`): Calls=["ParseJWT"], CalledBy=["AuthMiddleware", "RefreshToken"]
  - Chunk "GetUserClaims" (func, `internal/auth/service.go`): Calls=[], CalledBy=["AuthMiddleware"]
  - Chunk "ParseJWT" (func, `internal/auth/jwt.go`): Calls=[], CalledBy=["ValidateToken"]
  - Chunk "RefreshToken" (func, `internal/auth/service.go`): Calls=["ValidateToken", "GenerateToken"], CalledBy=["RefreshHandler"]
  - Chunk "GenerateToken" (func, `internal/auth/service.go`): Calls=[], CalledBy=["RefreshToken", "LoginHandler"]
  - Chunk "RegisterRoutes" (func, `internal/server/routes.go`): Calls=["AuthMiddleware"], CalledBy=[]
  - Chunk "LoginHandler" (func, `internal/auth/handler.go`): Calls=["GenerateToken"], CalledBy=["RegisterRoutes"]
  - Chunk "RefreshHandler" (func, `internal/auth/handler.go`): Calls=["RefreshToken"], CalledBy=["RegisterRoutes"]
  - Chunk "RateLimiter" (func, `internal/middleware/ratelimit.go`): Calls=[], CalledBy=["RegisterRoutes"] (unrelated to auth — verifies search specificity)
- [ ] Each chunk has a pre-computed embedding stored in LanceDB. Embeddings are generated such that auth-related chunks are closer together in vector space than the RateLimiter chunk. This can be achieved by:
  - Using the real embedding client against a running nomic-embed-code container (build-tag guarded), OR
  - Using synthetic vectors where auth-related chunks share a similar direction and RateLimiter points elsewhere

### Multi-Query Search Test

- [ ] Test: search with queries `["authentication middleware", "token validation"]`. Verify:
  - AuthMiddleware and ValidateToken appear in the top results (high relevance to both queries)
  - The test uses synthetic embedding vectors (not a real embedder) to guarantee deterministic results. Both top results have HitCount == 2. The synthetic embedder returns fixed vectors such that auth-related queries map near auth-related chunk embeddings, ensuring each query matches the same chunks with exact, reproducible scores.
  - Results are ordered by HitCount descending, then score descending
  - RateLimiter either does not appear or ranks near the bottom

### Hop Expansion Test

- [ ] Test: search with query `["validate authentication token"]` with ExpandHops=true. Verify:
  - ValidateToken is a direct hit
  - ParseJWT appears as a HopCallee of ValidateToken
  - AuthMiddleware appears as a HopCaller of ValidateToken
  - Hop results are tagged with correct HopRelation and HopSource

### Deduplication Across Direct Hits and Hops

- [ ] Test: search where AuthMiddleware is both a direct vector hit and a hop caller of ValidateToken. Verify AuthMiddleware appears exactly once in the results (as a direct hit, not duplicated as a hop).

### Budget Enforcement Test

- [ ] Test: search with MaxResults=5 and HopBudgetRatio=0.4 (directBudget=3, hopBudget=2). Verify total results do not exceed 5.

### No-Results Test

- [ ] Test: search with a query completely unrelated to the seeded data (e.g., `["kubernetes deployment yaml"]`). Verify the searcher returns results (it does not threshold-filter) but scores are low.

### Test Cleanup

- [ ] Temporary LanceDB directory is created via `t.TempDir()` and cleaned up automatically
- [ ] Test does not depend on external state — it seeds its own data and is self-contained
