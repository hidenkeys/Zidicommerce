# Decision log

| Date | Decision | Status |
|---|---|---|
| 2026-11-10 | Scope the first release as a controlled beta for one proposed merchant, Bing Chun, with designated testers and simulated fulfilment | Proposed for owner confirmation |
| 2026-11-10 | WhatsApp guided chat and grounded AI are the customer entry modes subject to acceptance. Keep native Flows and other customer channels disabled at launch | Release boundary |
| 2026-11-10 | Generate a transaction-specific checkout from the validated order. Require provider-verified payment for fulfilment and test mode across every checkout route | Release gate |
| 2026-11-10 | Keep the existing architecture. Defer provider selection to infrastructure evaluation with deterministic fallback included | Phase 4 decision pending |
| 2026-11-10 | Keep this phase documentation-only. Require explicit Phase 2 approval and separate written production approval | In effect |
| 2026-11-10 | Confirm Bing Chun, designated testers, test orders, Paystack test mode and simulated fulfilment | Approved for scope by owner |
| 2026-11-10 | Complete read-only Phase 2. Treat confirmed checkout defects and WhatsApp-only enforcement as stabilization blockers | Release promotion fails pending remediation |
| 2026-11-10 | Enforce beta restrictions behind `CONTROLLED_BETA`. Keep a durable checkout reservation and require operator reconciliation after uncertain initialization | Implemented in uncommitted candidate, review pending |
