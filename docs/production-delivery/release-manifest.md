# Release manifest

**Draft tracking record. No approved release candidate.**

| Item | Recorded value |
|---|---|
| Source at Phase 1 inspection | `e0019e6` on `codex/whatsapp-customer-experience` |
| Verified staging API revision | `0a3ac42`, Railway deployment `f5f0c459-6f0e-41a9-bf09-ededac06afaf` |
| Candidate commit, tag, Admin/API image digests | Commit `0a3ac42`. Release tag and image digests pending |
| Migration inventory | Local tail `000032`. Full inventory and environment state pending |
| Environment policy | WhatsApp only, Paystack test mode, native Flows disabled, unsafe seed and bypass settings disabled. Attestation pending |
| Release verification | CI, staging checkout retry and simulated pickup passed. Security checks and signed UAT pending |
| Backup and rollback | Pending restore rehearsal and immutable rollback targets |
| Production authorization | Pending explicit written approval |

Phase 2 scope confirmation covers Bing Chun, designated testers, Paystack test transactions and simulated fulfilment. No release candidate was approved. See the [audit backlog](phase-2-audit.md#prioritized-remediation-backlog) for stabilization gates.

Phase 3 added no migrations. Staging uses `CONTROLLED_BETA=true`. See the [Phase 3 release notes](phase-3-stabilization.md) for verification and remaining launch gates.
