# AI Regression Suite

This suite protects the Zidi AI customer-service layer before WhatsApp integration. It exercises the same `Service.Start` and `Service.Message` path used by the local admin AI chat, but runs against an isolated in-memory SQLite fixture by default.

The suite intentionally does not implement or call WhatsApp.

## What It Covers

- Hallucination prevention: invalid model output is rejected and replaced with deterministic grounded fallback answers.
- Tenant isolation: sessions and catalogue retrieval stay scoped to the current organization.
- Database as source of truth: products, variants, prices, stock, stores, orders, and FAQs are read from the database.
- Product and FAQ accuracy: answers must include seeded authoritative facts and exclude facts from other tenants.
- Multi-turn customer conversations: follow-up messages like "it" and "that" resolve from conversation state.
- DB-change freshness: price, inventory, and FAQ changes are visible on later turns without rebuilding a vector index.
- Merchant knowledge grounding: structured policy/FAQ/business-info rows are tenant-scoped and update immediately.
- Output hygiene: customer responses must not leak JSON, tool names, UUIDs, SQL details, prompts, API keys, or access tokens.

## Default Deterministic Run

From the API directory:

```bash
cd apps/api
go test ./internal/ai -run 'TestAIRegression'
```

This default run does not require Ollama, Groq, network access, or a live Postgres database. The test model intentionally returns bad output, so the suite verifies grounding, validation, and fallback behavior deterministically.

To run all API tests:

```bash
cd apps/api
go test ./...
```

## Live Provider Smoke Tests

Live checks are opt-in because they call a real model and can be slower or rate-limited.

Ollama:

```bash
cd apps/api
AI_REGRESSION_LIVE=1 \
AI_REGRESSION_PROVIDER=ollama \
OLLAMA_BASE_URL=http://127.0.0.1:11434 \
OLLAMA_CHAT_MODEL=llama3.2:3b \
go test ./internal/ai -run 'TestAIRegressionLiveProvidersSmoke'
```

Groq:

```bash
cd apps/api
AI_REGRESSION_LIVE=1 \
AI_REGRESSION_PROVIDER=groq \
GROQ_API_KEY=... \
GROQ_BASE_URL=https://api.groq.com/openai/v1 \
GROQ_MODEL=openai/gpt-oss-20b \
go test ./internal/ai -run 'TestAIRegressionLiveProvidersSmoke'
```

Both providers:

```bash
cd apps/api
AI_REGRESSION_LIVE=1 AI_REGRESSION_PROVIDER=both go test ./internal/ai -run 'TestAIRegressionLiveProvidersSmoke'
```

## Live Provider Evaluation Harness

Use the evaluation harness when you want a comparison report across the same realistic customer-service suite. It runs product, FAQ, hallucination, tenant-isolation, multi-turn, security, and DB-freshness cases against the selected live provider or providers.

Markdown report for both Ollama and Groq:

```bash
cd apps/api
AI_EVAL_LIVE=1 \
AI_EVAL_PROVIDER=both \
AI_EVAL_REPORT_FORMAT=markdown \
AI_EVAL_REPORT_PATH=ai-provider-eval-report.md \
go test ./internal/ai -run 'TestAIRegressionLiveProviderEvaluationHarness' -count=1 -v
```

JSON report:

```bash
cd apps/api
AI_EVAL_LIVE=1 \
AI_EVAL_PROVIDER=both \
AI_EVAL_REPORT_FORMAT=json \
AI_EVAL_REPORT_PATH=ai-provider-eval-report.json \
go test ./internal/ai -run 'TestAIRegressionLiveProviderEvaluationHarness' -count=1 -v
```

Provider selection:

```text
AI_EVAL_PROVIDER=ollama
AI_EVAL_PROVIDER=groq
AI_EVAL_PROVIDER=both
```

The harness reads provider configuration from the environment. If a variable is missing, it will also read `.env`, `../.env`, or `../../.env` relative to the test working directory.

```text
OLLAMA_BASE_URL=http://127.0.0.1:11434
OLLAMA_CHAT_MODEL=llama3.2:3b
GROQ_API_KEY=...
GROQ_BASE_URL=https://api.groq.com/openai/v1
GROQ_MODEL=openai/gpt-oss-20b
```

By default, the evaluation test writes the report and lets the Go test process pass, even when providers fail cases. This is useful for side-by-side model comparison. To make the test exit non-zero when any selected provider has failed cases or provider errors:

```bash
AI_EVAL_FAIL_ON_FAILURE=1
```

The report includes:

- pass/fail counts per provider
- hallucination failure counts
- tenant-isolation failure counts
- failures grouped by category
- average response time per provider
- failed response excerpts for diagnosis

## Adding Cases

Add deterministic cases to `apps/api/internal/ai/regression_suite_test.go`.

Use `newAIRegressionFixture` when a case needs seeded commerce data. It creates:

- `StrideStreet Shoes`, a shoes tenant with stores, products, inventory, FAQs, and a customer order.
- `GlowNest Cosmetics`, a separate cosmetics tenant used to prove cross-tenant isolation.

Prefer asserting on customer-visible facts rather than exact full sentences:

```go
response := f.send(t, f.orgA, sessionID, "How much is the black size 42 Urban Runner Sneakers?")
assertContainsAll(t, response.Message.Body, []string{"Urban Runner Sneakers", "Black Size 42", "NGN 28500"})
assertContainsNone(t, response.Message.Body, []string{"GlowNest", "Radiance Vitamin C Serum"})
```

Use live-provider tests only for small smoke coverage. Broad regression coverage should stay deterministic so it is stable in CI and does not depend on model availability.

Phase D merchant knowledge architecture and entry examples are documented in `docs/merchant-knowledge.md`.
