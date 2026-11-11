# Controlled beta readiness checklist

Execution started November 11, 2026. Release and operations owner is Oluwateniola Sobande. Historical records remain dated evidence. This checklist tracks the current candidate.

| Gate | State | Evidence or remaining work |
|---|---|---|
| Payment controls and authorization | Proven | Phase 3 tests and PR #1 |
| Staff disablement and demotion | Proven | PR #2 and staging tokens rejected |
| Background job tenant isolation | Proven | PR #3 and six probes on 08f5738 |
| Checkout concurrency and ambiguous references | Proven | Phase 3 staging probes |
| Provider payment and first webhook delivery | Proven | ZC-20261110-88697F27 |
| Previously unseen concurrent webhook | Proven | Six simultaneous signed requests, one paid transition |
| Signature rejection and sampled secret scan | Proven within recorded scope | Phase 3 evidence |
| Guided device checkout and simulated pickup | Proven historically | Phase 3 staging orders |
| Immutable candidate with tester filter | In progress | Candidate branch based on 08f5738 |
| Safe concurrent migration startup | Open | Inspect runner, fix and exercise PostgreSQL |
| Migration inventory and clean installation | Open | All 32 migrations |
| Backup restoration and application rollback | Open | Disposable restore and recorded rollback artifact |
| Meta identity, signatures, token and routing | Open | Reattest existing staging phone override |
| Grounded live AI and fallback | Open | Current provider configuration and deployed tests |
| Handoff, claim, reply, release and resumption | Open | Deployed API and UI |
| Admin desktop/mobile and role acceptance | Open | Administrator, manager, support and viewer |
| Cancellation, inventory and invalid transitions | Open | Reuse tests and close deployed evidence |
| Channel policy, opt-out and service window | Open | Deployed negative and approved template paths |
| Monitoring and alert delivery | Open | Existing destination, controlled alert |
| Complete verification suite | Open | API, race, builds, journeys and readiness script |
| Final release manifest and diff | Open | Full commit and staging deployment IDs |
| Signed UAT and production decision | Pending owner | Consolidated final decision only |
