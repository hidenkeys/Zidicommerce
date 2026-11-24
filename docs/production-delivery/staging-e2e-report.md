# Staging end-to-end evidence report — 24 November 2026

## 1. Summary

**Verdict: partial acceptance; full handoff scope remains incomplete.** Bing Chun has recorded paid pickup orders, completed fulfilment, cancellation, handoff recovery, knowledge approval controls and clean post-fix responsive captures. The second workspace has configuration and preview coverage, but real WhatsApp and commerce are blocked by the shared number’s existing tenant binding.

Deployment identifiers supplied for this review: **API `e6487d09`; Admin `ee409715`**. The evidence files do not map every earlier run to a deployment; these identifiers must not be read as proof that every capture used those exact builds. No network verification was performed for this report.

Fixes inspected with `git show`: `e6fe990` fixes WhatsApp policy requests for non-WhatsApp inbox sessions; `cdd6a3e` preserves store configuration during detail edits and exposes explicit hours editing/removal; `4011b52` lets released journeys restart from published entry keywords instead of falling into AI. See section 4 for regression coverage.

Evidence rules: PASS applies only to the stated check. FAIL is an observed unsuccessful check; NOT RUN means no adequate recorded execution; BLOCKED means a known prerequisite prevents execution. A script’s broad PASS label does not prove all words in its test name: for example, an unbound preview does not prove activation. Later successful retries supersede earlier failures only for the same operation. Scripts were read, not executed. All evidence paths below are repository-relative; `E/` means `.staging-evidence/2026-11-24-`.

Organizations: **BC = Bing Chun Nigeria (Staging)** (`ece90dea-cd44-4a76-adfa-bda9783e7846`); **BD = Bing Chuan Desserts (Staging)** (`5b198c84-ab62-452c-ba49-0f8faff5f708`). Workspace-wide checks are not repeated as store-level successes.

## 2. Results matrix

Matrix-cell totals (scope, mode and store tables combined; overlapping checks, not unique test cases): **33 PASS / 1 FAIL / 38 NOT RUN / 11 BLOCKED**. Historical retries and the evidence index are excluded.

### Handoff scope 1–10

| Scope | Organization / store / mode | Result | Evidence | Recorded outcome / limit |
|---|---|---|---|---|
| 1 Auth | BC / workspace / all | PASS | `E/interactive-final` | Owner sign-in and tenant snapshot. |
| 1 Auth | BD / workspace / all | PASS | `E/second-modes` | Owner sign-in identifies the separate tenant. |
| 1 Invitation send | BC / workspace / all | PASS | `E/email` | HTTP 200, email status `sent`; not independent inbox-delivery proof. |
| 1 Invitation send | BD / workspace / all | PASS | `E/second-invitation` | HTTP 200, email status `sent`; not independent inbox-delivery proof. |
| 1 Invitation delivery/acceptance | BC / workspace / all | NOT RUN | `E/email`, `E/staff-access`; `user-testing-guide.md` | Guide says invitation accepted; artifacts show staff UI access but no acceptance transaction or inbox receipt. |
| 1 Invitation delivery/acceptance | BD / workspace / all | NOT RUN | `E/second-invitation` | Send recorded; delivery/acceptance not independently recorded. |
| 1 Reset request | BC staff / workspace / all | PASS | `E/reset-after-acceptance` | HTTP 200 and generic acknowledgement only. |
| 1 Reset delivery/completion | Both / workspace / all | NOT RUN | `E/reset-after-acceptance` | Explicitly requires inbox confirmation; no delivered message or completed reset recorded. |
| 1 Staff UI restrictions | BC / workspace / all | PASS | `E/staff-access` | Payments/team read-only; Channels and Flow Studio access denied. |
| 1 Staff API denials | Both / workspace / all | NOT RUN | `E/staff-access` | No recorded HTTP 403 assertions for payments/team/channels/publish; UI text is insufficient. |
| 1 Staff UI restrictions | BD / workspace / all | NOT RUN | `E/second-invitation` | Invitation send does not prove role enforcement. |
| 2 Catalogue/inventory/store CRUD | BC / fixture store / all | PASS | `E/crud` | Category/product/variant/price, store and stock forms round-trip. |
| 2 Catalogue/inventory/store CRUD | BD / fixture store / all | PASS | `E/second-crud` | Same recorded form round-trips; named kitchen coverage below. |
| 2 Store hours | BC / Staging QA Store / all | PASS | `E/store-hours-final`, `E/hours-layout-final` | Monday 08:30 edit, removal, detail-only preservation and fulfilment preservation. |
| 3 Knowledge | BC / workspace / AI Conversation | PASS | `E/knowledge-recovery` | UI ingestion/review/approval/archive and AI test-chat gating; not a real WhatsApp knowledge test. |
| 3 Knowledge | BD / workspace / AI Conversation | PASS | `E/second-knowledge` | Same lifecycle and test-chat gating in second tenant. |
| 4 Number workspace/test | BC / workspace / all | PASS | `E/interactive-final`, `E/number-test`, `E/channel-completion`, connectivity JSON | Overview/Experience/Test/Health/Settings; confirmed tester-only send accepted; setup healthy. |
| 4 Capability status consistency | BC / workspace / all | FAIL | `E/channel-completion` | Healthy setup coexists with many `setup_required` capabilities; complete hub accuracy is not established. |
| 4 Wizard/manual path | Both / workspace / all | PASS | `E/interactive-final`, `E/second-channel-recovery` | Embedded Signup unavailable with reason; manual recovery path visible. This does not prove second connection configured. |
| 4 Second number connectivity | BD / workspace / all | BLOCKED | `E/second-channel-recovery` | Connection exists, `configured:false`; shared number still belongs to BC. |
| 5 Modes | Both / workspace / each mode | BLOCKED | Mode matrix below | Editor/preview successes do not complete required real-channel coverage for every new experience. |
| 6 Paid pickup orders | BC / Jara, Lekki, QA / recorded modes | PASS | Paid-order ledger below | Six distinct paid orders reach completed order/fulfilment state; not all stores or modes covered. |
| 6 Cancellation | BC / Staging QA Store / mode unrecorded | PASS | `E/cancellation` | `ZC-20261124-7A43B0E3` cancelled; on-hand 23→24, reserved 0→0; stock restored. No nonzero reservation decrease shown. |
| 6 Commerce | BD / all stores / all four modes | BLOCKED | `E/second-readiness`, `E/second-channel-recovery` | Paystack test configuration active; no paid second-tenant order recorded. |
| 7 Handoff/release | BC / workspace / Guided Questions entry | PASS | `E/explicit-handoff`, `E/guided-handoff-regression`, `E/operations-retry` | `human_requested`→`ai_handling` recorded; exact staff reply below. |
| 7 Handoff | BD / workspace / all modes | BLOCKED | `E/second-channel-recovery` | No connected real number; no second-tenant handoff run recorded. |
| 8 Field | Both / all stores / all | BLOCKED | `codex-e2e-handoff.md`, `user-testing-guide.md` | Field not deployed on staging; local-suite claims are not staging results. |
| 9 Onboarding | BC / workspace / all | NOT RUN | `E/baseline`, `E/postfix-original` | Tour first screen shown, but no full fresh-tenant eight-step acceptance recorded for BC. |
| 9 Onboarding | BD / workspace / all | PASS | `E/second-profile`, `E/second-catalogue` | Eight tour titles recorded; later readiness and Channels next-step empty state verified. Initial heading timeout recovered. |
| 10 Responsive | BC / workspace / all | PASS | `E/postfix-original`, `E/hours-layout-final` | 60 route/panel captures at 390/768/1440 clean; hours editor outer/interior widths fit. |
| 10 Responsive | BD / workspace / all | PASS | `E/postfix-second` | 60 route/panel captures at 390/768/1440 clean after inbox fix; not every possible interactive state. |

### Scope 5: mode-specific coverage

| Organization / mode | Result | Evidence | One-line outcome |
|---|---|---|---|
| BC / Automated Commerce | PASS | `E/original-mode-edits`, `E/mode-activation` | New `9ef0fde0-f7fc-4bd1-a777-cc548ceb31d1` edited/validated/published/previewed; activated keyword `qaauto`; default unchanged. |
| BC / Guided Questions | PASS | `E/original-mode-edits`, `E/mode-activation` | New `f92fa879-a3e1-432e-b77c-2a9d177dfbbc` edited/validated/published/previewed; activated `qaguided`. |
| BC / AI Conversation | PASS | `E/original-mode-edits`, `E/mode-activation` | New `1dd4e216-1e51-4da9-9479-687503827088` edited/validated/published/previewed; activated `qaai`. |
| BC / new QA keywords on device | NOT RUN | `E/mode-activation` | Activation assertions do not record tester messages/replies for `qaauto`, `qaguided`, `qaai`. |
| BC / WhatsApp Flow `qaflow` | PASS | `E/native-flow-retry`, `E/native-paid-order` | Bound/validated/provider-published/Zidi-published; real form response and paid completed Jara order recorded. |
| BC / WhatsApp Flow `qascratch` build/preview | PASS | `E/scratch-flow`, `E/scratch-published`, `E/shared-flow-preview-final` | Blank-built question→Finish, provider and Zidi publication; shared Chat/Flow simulation completes with `Strawberry Sundae`. |
| BC / WhatsApp Flow `qascratch` device completion | NOT RUN | Same folders; `user-testing-guide.md` | Guide asserts real completion; supplied artifacts independently record simulation/publication, not a device transcript. |
| BD / Automated Commerce | PASS | `E/second-modes` | `44d64e7a-37be-4f8a-98f8-40d9de97b774` edited/validated/published/previewed, explicitly not activated. |
| BD / Guided Questions | PASS | `E/second-modes` | `2f4ffa19-5b99-48b3-996f-63dae3c6755d` edited/validated/published/previewed, explicitly not activated. |
| BD / AI Conversation | PASS | `E/second-modes` | `c76de135-872b-43e3-a4f4-4293b0475ee5` edited/validated/published/previewed, explicitly not activated. |
| BD / WhatsApp Flow publication/activation | BLOCKED | `E/second-modes` | `50c955dc-d7e8-42b3-9738-494eccaf7d2f` edited/validated/previewed; provider publication not performed without connection. |
| BD / all four modes activation/device | BLOCKED | `E/second-modes`, `E/second-channel-recovery` | No real tenant-bound number; no device messages or replies recorded. |

### Scope 2 and 6: per-store coverage

The four mode columns below concern **paid commerce completion**, not editor coverage. AI Conversation is a grounded-answer mode; the guide routes writes to `order`, so no separate AI-mode paid order is inferred. Folder names identify ordering modes; the order-check script itself asserts payment/pickup/transition state, not the initiating mode. `Kitchen edited…` is a CRUD fixture. Inactive stores were intentionally preserved. The second-CRUD result omits its store name; no additional distinct BD store is inferred.

| Organization / store | Store/stock evidence | Automated Commerce | Guided Questions | WhatsApp Flow | AI Conversation |
|---|---|---|---|---|---|
| BC / Bingchun Jara Mall | `E/every-active-store`: hours preserved, fixture stock 60 | PASS | PASS | PASS | NOT RUN |
| BC / Bing Chun Lekki | `E/every-active-store`: hours preserved, fixture stock 60 | PASS | PASS | NOT RUN | NOT RUN |
| BC / Staging QA Store | `E/qa-store-final`, `E/hours-layout-final`: stock 60, hours recovery | NOT RUN | PASS | NOT RUN | NOT RUN |
| BC / Kitchen edited 1795493132431 | `E/crud`, `E/every-active-store`: hours preserved, fixture stock 60 | NOT RUN | NOT RUN | NOT RUN | NOT RUN |
| BC / Bing Chun CURATE | `E/every-active-store`: inactive, read-only | NOT RUN | NOT RUN | NOT RUN | NOT RUN |
| BC / Bing Chun Victoria Island | `E/every-active-store`: inactive, read-only | NOT RUN | NOT RUN | NOT RUN | NOT RUN |
| BC / Bing Chun Festac | `E/every-active-store`: inactive, read-only | NOT RUN | NOT RUN | NOT RUN | NOT RUN |
| BC / Bing Chun Surulere | `E/every-active-store`: inactive, read-only | NOT RUN | NOT RUN | NOT RUN | NOT RUN |
| BC / Bing Chun Maryland | `E/every-active-store`: inactive, read-only | NOT RUN | NOT RUN | NOT RUN | NOT RUN |
| BD / Bing Chuan Lekki Kitchen | `E/second-readiness`, `E/second-final-catalogue`: dessert catalogue/stock, test payments | BLOCKED | BLOCKED | BLOCKED | BLOCKED |

### Paid-order ledger

All references below are recorded as Paystack paid pickup orders; each ends with order **completed**, fulfilment **completed**, after processing and ready. This proves persisted paid state, not an independently captured webhook signature for every payment. The native transcript additionally records customer payment confirmation and status notifications. Store identity for the otherwise unnamed `automated-paid-order` and `guided-paid-order` runs was checked in their `paid-order.png` screenshots.

| Organization / store / mode | Order | Paystack test transaction reference | Evidence |
|---|---|---|---|
| BC / Jara / WhatsApp Flow `qaflow` | `ZC-20261124-A39F9417` | `zc_8ab7fc31-6492-45d4-bc0c-130a1fdac721` | `E/native-paid-order` |
| BC / Jara / Guided Questions | `ZC-20261124-7C09BC6C` | `zc_04644096-8272-4874-8782-06a01d5cbb9d` | `E/guided-paid-order` |
| BC / Lekki / Automated Commerce | `ZC-20261124-42AB71C8` | `zc_1e5fe682-8926-4f42-a1aa-8d2139b852dd` | `E/automated-paid-order` |
| BC / Jara / Automated Commerce | `ZC-20261124-BCC62611` | `zc_3bbd9079-74ca-48bd-9dcd-30f1fd26235c` | `E/automated-jara-paid-order` |
| BC / Lekki / Guided Questions | `ZC-20261124-241CF9A8` | `zc_bdae114c-b1c4-4aba-8f5e-1a9eb3b35c83` | `E/guided-lekki-paid-order` |
| BC / Staging QA Store / Guided Questions | `ZC-20261124-8CFA1A0D` | `zc_a07de604-427d-41b8-861b-7b0ff0e6a58a` | `E/guided-qa-paid-order`, `E/guided-qa-resumed` |

The last run initially failed with `fetch failed` after the paid assertion; the resumed run records processing already reached, then ready and completed. Cancellation is a separate unpaid order, not one of these six.

### Exact recorded messages and replies

Device transcript: `E/native-paid-order/whatsapp-transcript.txt`, tester ending **2209**, business number ending **5382**. The following excerpts preserve message wording; sensitive recipient details, checkout URL and pickup code are omitted.

| Sent / action recorded | Reply observed |
|---|---|
| `qaflow` | “Continue your request securely in WhatsApp.” with **Open order form**. |
| Native form response (**Response sent** recorded; individual field answers absent from transcript) | “Order created. Please complete payment to confirm it.” Order `ZC-20261124-A39F9417`, Awaiting payment, Grape Smoothie x1, NGN 4200, Bingchun Jara Mall, Pickup. |
| Payment link issued | “Use this secure payment link to complete your order: [checkout URL omitted] After payment, we will confirm your order automatically.” Then “Use the secure link above, then check payment status.” |
| Payment completed; exact customer payment-check input not recorded | “Payment confirmed for order ZC-20261124-A39F9417.” with store and pickup code; then paid order summary, “We will keep you updated here.” |
| Form/payment continuation | “Your verified payment and order are confirmed.” Then “Thank you. You can send menu to start again.” |
| Admin order transitions | “Order ZC-20261124-A39F9417 is now processing.” / “Order ZC-20261124-A39F9417 is now ready.” with pickup instructions / “Order ZC-20261124-A39F9417 is now completed.” |
| `guided` | “Welcome to Bing Chun. Follow these questions to place your staging test order.” Then “How can we help today?” with **Place an order**, **Talk to staff**. |
| Earlier human request (exact initiating input absent) | “A team member will help you with this request.” |
| Staff reply, recorded both in script and device transcript | “Zidi staging support test: your request reached the team. I am returning this conversation to automation.” |

The guide gives `order` → “Welcome to Bing Chun. Let’s place your staging test order.” and `menu` then `What is the price of one Grape Smoothie at Bingchun Jara Mall?` → NGN 4,200. Those are guide-reported expectations/observations, not independently captured exact device exchanges in the supplied text/JSON. No exact reply is invented for `qaauto`, `qaguided`, `qaai`, or second-workspace modes. Cancellation JSON records state/stock, not a WhatsApp cancellation exchange.

Scratch Flow: script configures “Which dessert would you like to try on your next visit?”; shared-preview JSON records answer `Strawberry Sundae`, completed, screen `FINISH`. The guide’s assertion of device completion and final acknowledgement has no corresponding device transcript in these folders.

Knowledge checks use **Admin AI test chat**, not WhatsApp. BC question: “What is the acceptance verification code for QA session 1795493546989?” Before approval and after archive: “I’m sorry, but the merchant has not provided that information.” Approved: “The acceptance verification code for QA session 1795493546989 is ZA1795493546989. Reviewed during staging acceptance.” BD repeats the same sequence with session/code `1795494675915` / `ZA1795494675915` and otherwise identical wording. This establishes gating for these test facts, not universal AI correctness.

## 3. Shared-number routing

Last recorded binding for ZidiHQ **…5382** is BC connection **`0dcec1fe-f469-43b2-9a62-a0f92836f1e7`**; phone asset ID `683059678235387`, WABA `773193152076286`. Default remains **Bing Chun — AI Conversation**, journey `813b8d1b-36c8-4cec-9941-f07645f19739`, version `ddb75e8b-3fed-47c8-b0a5-0b49369f2295`. The activation checks explicitly preserve it. Keywords route individual published journeys; they do not move the phone between tenants.

Current callback:
`https://zidicommerce-staging-api-staging.up.railway.app/v1/runtime/webhooks/whatsapp?connection_id=0dcec1fe-f469-43b2-9a62-a0f92836f1e7`

BD manual-recovery connection **`ccdb0882-9b58-4633-ba83-d1cd9abceb11`**, “Bing Chuan staging tester connection”, exists but is **not configured**. Its callback is:
`https://zidicommerce-staging-api-staging.up.railway.app/v1/runtime/webhooks/whatsapp?connection_id=ccdb0882-9b58-4633-ba83-d1cd9abceb11`

Other connections in evidence: `6581ea3f-5aa5-4728-be31-411a940a7b87` is the older BC staging-test pilot binding; `83767306-cb7b-4fdc-8466-444b0ff6c036` appears in the second tenant’s erroneous non-WhatsApp policy request. Neither is evidence that the shared phone moved to BD.

Required operator/Meta procedure (pending; not performed by this documentation task):

1. Owner approves temporary cross-tenant use of the shared asset. Record the BC connection/default above and preserve the restoration plan. Confirm the target tenant and designated tester ending 2209.
2. Through the supported Channels manual/operator recovery flow, authorize/configure BD connection `ccdb0882-9b58-4633-ba83-d1cd9abceb11` with the approved phone/WABA assets and server-side credentials. Resolve any existing-asset ownership conflict through supported operator flows; the evidence does not establish that duplicate assignment is permitted.
3. The authorized Meta operator changes the **phone-level webhook override** for phone ID `683059678235387` to the BD callback above; retain the required WABA subscription and verify the callback using the securely configured verification material. Do not substitute the unrelated test-channel connection.
4. Refresh Health, perform the confirmed tester-only outbound test, send a fresh inbound message, verify signed routing into BD, complete validation, then publish/activate the BD experiences on that connection and execute missing device/payment/handoff checks.
5. Restore the asset binding and phone-level override to the BC callback, restore/confirm the original AI default, and prove restoration with a fresh inbound tester message and health check.

The handoff explicitly requires stopping for Meta-side callback changes. No approved move or Meta override execution is recorded; therefore second-workspace real WhatsApp was not run. Final connectivity JSON shows HTTP 200, setup healthy, inbound `08:35:49Z` and signed event `08:35:55Z`. The later runtime snapshot still belongs to BC and records an active `qaflow` store prompt at `09:22:42Z`; this is not a fresh successful order or a restoration-after-move test.

## 4. Bugs found and fixed; unresolved findings

| Commit | Fix and regression additions | Staging evidence / limit |
|---|---|---|
| `e6fe990` | Projects provider from tenant-owned channel into conversation DTO/list/detail; requests messaging policy only for WhatsApp. Adds `TestConversationProviderComesFromTenantChannel` (including foreign-tenant denial) and `e2e/conversation-provider.mjs` (test vs WhatsApp composer/policy behavior). | `E/second-404-segments` identifies erroneous policy-context 404; `E/postfix-original` and `E/postfix-second` are clean, 60 captures each. |
| `cdd6a3e` | Detail-only store saves preserve hours/fulfilment/coordinates; explicit edit/remove hours controls. Adds `e2e/store-hours.mjs` for detail save, hours edit and removal. | `E/store-hours-final`, `E/hours-layout-final`: preserved details/fulfilment and responsive hours editor. |
| `4011b52` | Published entry keyword bypasses post-release AI fallback; natural questions still reach AI. Adds `TestReleasedGuidedJourneyEntryBypassesAI` subcases `guided` and `dessert_help`. | `E/guided-handoff-regression` records handoff/release; native transcript records guided entry response. JSON alone does not assert every new Go regression ran. |

Commit diffs prove tests were **added**, not that the full build/test suite ran successfully. No suite execution logs are present in the specified evidence, so no full-suite pass is claimed.

Unresolved finding: capability records still report provider onboarding/asset verification required despite healthy setup and successful real traffic. `commerce_actions`, `human_handoff`, `inbound_text`, media, profile, receipts, templates and OAuth onboarding remain `setup_required`; only `interactive_messages` and `outbound_text` are `available` in `E/channel-completion`. The earlier **Needs attention** hub warning cannot be declared cleared merely from setup health; no later reconciled capability snapshot is supplied.

Other unsuccessful runs are preserved, without assuming every timeout is a product defect: initial Flow/AI editor locators; native settings close locator; shared-preview log locator; knowledge chunk/review locators with dependent approval/archive assertions; second workspace readiness/Operator recovery heading locators; QA-store undefined `.map`; and QA-order `fetch failed`. Later scoped successes are indexed below. There is no evidence of an additional confirmed, unfixed product-code bug beyond the capability/readiness discrepancy; absent coverage remains an acceptance gap.

## 5. Remaining blockers and owner actions

1. **Meta owner/operator:** configure `META_EMBEDDED_SIGNUP_CONFIGURATION_ID`; complete the required App Review/provider onboarding and asset/scope verification. The wizard’s unavailable state is proven; App Review completion is not. Reconcile capability records and confirm whether ZidiHQ still displays Needs attention.
2. **Business owner + Meta operator:** approve and perform the temporary shared-number move and callback sequence in section 3, then run all four BD modes, payments, cancellation and handoff; restore BC and prove inbound routing.
3. **QA + authorized inbox owner:** confirm invitation/reset delivery, record invite acceptance/reset completion, and capture staff API 403 responses plus BD UI denials. `sent` and generic HTTP 200 are insufficient delivery evidence.
4. **QA:** record device exchanges for the three new QA keywords and scratch-built `qascratch`; complete missing per-store/mode commerce checks. Preserve the distinction between AI answers and protected order execution. Capture any required interactive responsive states absent from the route sweep.
5. **Deployment owner:** deploy Field to staging if staging dispatch/job acceptance is required; otherwise retain it as BLOCKED here. Local tests do not substitute for staging evidence.

## 6. Evidence index

Every dated folder is listed, including empty/debug folders and failed attempts. `results.json` is the default artifact; UI runs use `summary.json`/`summary.md`. Raw evidence is local/gitignored and can contain private data; this report deliberately omits credentials, personal addresses and full phone numbers.

| Folder / artifact | What it proves (and limits) |
|---|---|
| `.staging-evidence/2026-11-24-automated-jara-paid-order/` | Jara paid pickup and completed order/fulfilment; transaction in ledger. |
| `.staging-evidence/2026-11-24-automated-paid-order/` | Lekki paid pickup and completed order/fulfilment; store resolved from paid-order screenshot. |
| `.staging-evidence/2026-11-24-baseline/` | 63 baseline captures, no recorded error/overflow flags; older harness temporarily revealed desktop journey navigation. |
| `.staging-evidence/2026-11-24-cancellation/` | Identified unpaid QA order cancelled and on-hand restored. |
| `.staging-evidence/2026-11-24-channel-completion/` | Setup validation healthy; capability snapshot still has setup_required entries. |
| `.staging-evidence/2026-11-24-conversations-diagnosis/` | 15 diagnostic captures; no errors in this particular run, not proof against later 404 reproduction. |
| `.staging-evidence/2026-11-24-crud/` | BC catalogue/store/inventory form round-trips. |
| `.staging-evidence/2026-11-24-current-ui/` | 60 captures; Conversations 404 at 768/1440 before fix, no overflow. |
| `.staging-evidence/2026-11-24-editor-debug/` | Editor screenshots only; no structured pass/fail result. |
| `.staging-evidence/2026-11-24-email/` | BC invitation API send reports sent; no inbox receipt. |
| `.staging-evidence/2026-11-24-every-active-store/` | Hours preservation/fixture stock on three active stores; inactive stores read-only; QA store map error. |
| `.staging-evidence/2026-11-24-explicit-handoff/` | Healthy connection; human_requested to ai_handling after staff workflow. |
| `.staging-evidence/2026-11-24-guided-handoff-regression/` | Guided support request/reply/release state recovery. |
| `.staging-evidence/2026-11-24-guided-lekki-paid-order/` | Lekki paid pickup through completed fulfilment. |
| `.staging-evidence/2026-11-24-guided-paid-order/` | Jara paid pickup through completed fulfilment; store resolved from screenshot. |
| `.staging-evidence/2026-11-24-guided-qa-paid-order/` | QA order paid; transition run interrupted by fetch failed. |
| `.staging-evidence/2026-11-24-guided-qa-resumed/` | Same QA order resumed from processing through ready/completed. |
| `.staging-evidence/2026-11-24-harness-recovery/` | Health and staff handoff/release pass after earlier harness failure. |
| `.staging-evidence/2026-11-24-hours-layout-final/` | Explicit hours operations preserved state; outer and interior widths fit 390/768/1440. |
| `.staging-evidence/2026-11-24-interactive/` | Early tenant/binding and number navigation snapshot; partial mode workflow attempts. |
| `.staging-evidence/2026-11-24-interactive-final/` | Tenant/binding snapshot, number tabs/health, unavailable signup reason; partial mode runs with Flow/AI locator failures. |
| `.staging-evidence/2026-11-24-knowledge/` | Initial ingestion and unapproved refusal; approval/archive locator failures and dependent assertion failures. |
| `.staging-evidence/2026-11-24-knowledge-final/` | New ingestion/unapproved refusal; review/archive still failed before recovery. |
| `.staging-evidence/2026-11-24-knowledge-recovery/` | BC reused ingestion, approved fact answer and archived fact refusal. |
| `.staging-evidence/2026-11-24-mode-activation/` | Three new BC QA experiences activated; default unchanged, no device exchange assertion. |
| `.staging-evidence/2026-11-24-mode-diagnosis/` | No JSON/TXT summary evidence; no acceptance result inferred. |
| `.staging-evidence/2026-11-24-mode-recovery/` | AI editing/publication/preview recovered; Flow publication still returned 400. |
| `.staging-evidence/2026-11-24-native-flow/` | Initial settings-close locator timeout. |
| `.staging-evidence/2026-11-24-native-flow-retry/` | qaflow bound/validated/provider-published/Zidi-published; real Flow screenshots supplement transcript. |
| `.staging-evidence/2026-11-24-native-paid-order/` | qaflow device transcript, payment confirmation and status notifications; paid/completed pickup assertions. |
| `.staging-evidence/2026-11-24-number-test/` | Confirmed controlled tester-only send, HTTP 200/accepted. |
| `.staging-evidence/2026-11-24-operations/` | Healthy after tester inbound; handoff response-wait timeout. |
| `.staging-evidence/2026-11-24-operations-retry/` | Staff workflow returns human_assigned to ai_handling. |
| `.staging-evidence/2026-11-24-original-mode-edits/` | Three new BC modes edited/validated/published/previewed without activation in this run. |
| `.staging-evidence/2026-11-24-postfix-original/` | 60 BC route/panel captures clean at 390/768/1440. |
| `.staging-evidence/2026-11-24-postfix-second/` | 60 BD route/panel captures clean at 390/768/1440. |
| `.staging-evidence/2026-11-24-qa-store-final/` | QA store retry: hours preserved, isolated fixture stock 60. |
| `.staging-evidence/2026-11-24-reset-after-acceptance/` | Reset request HTTP 200; delivery explicitly unconfirmed. |
| `.staging-evidence/2026-11-24-runtime-final/` | Designated tester snapshot: BC WhatsApp session, qaflow store prompt, prior cancelled order; not a full transcript. |
| `.staging-evidence/2026-11-24-scratch-flow/` | Blank-built required text question followed by Finish, valid. |
| `.staging-evidence/2026-11-24-scratch-published/` | qascratch binding/validation/provider and Zidi publication; no device completion assertion. |
| `.staging-evidence/2026-11-24-second-404-detail/` | Second-tenant Conversations fetch 404 before fix. |
| `.staging-evidence/2026-11-24-second-404-segments/` | Identifies erroneous WhatsApp policy-context request for second-tenant test-channel session. |
| `.staging-evidence/2026-11-24-second-catalogue/` | Dessert product photographs, readiness card and Channels next-step empty state. |
| `.staging-evidence/2026-11-24-second-channel/` | Initial Operator recovery heading timeout. |
| `.staging-evidence/2026-11-24-second-channel-recovery/` | Manual recovery path and target callback visible; configured false. |
| `.staging-evidence/2026-11-24-second-crud/` | BD catalogue/store/inventory form round-trips. |
| `.staging-evidence/2026-11-24-second-final-catalogue/` | Fruit Smoothies/Fruit tea/Sundaes categorization and three named dessert products. |
| `.staging-evidence/2026-11-24-second-invitation/` | BD invitation send reports sent; no independent delivery/acceptance. |
| `.staging-evidence/2026-11-24-second-knowledge/` | BD ingest/review/approve/archive and test-chat gating. |
| `.staging-evidence/2026-11-24-second-modes/` | Four editor/validation/preview workflows; three published unbound; native publication blocked. |
| `.staging-evidence/2026-11-24-second-org/` | Separate organization created; initial tour wait failed; login file is private and not reproduced. |
| `.staging-evidence/2026-11-24-second-profile/` | Eight tour titles and renamed profile; initial readiness heading wait failed. |
| `.staging-evidence/2026-11-24-second-readiness/` | Kitchen stock for tea/sundae and active Paystack test configuration; no payment execution. |
| `.staging-evidence/2026-11-24-second-responsive/` | 60 captures; Conversations 404 at all three widths before fix, no overflow. |
| `.staging-evidence/2026-11-24-shared-flow-preview/` | Initial preview-log locator timeout. |
| `.staging-evidence/2026-11-24-shared-flow-preview-final/` | Scratch shared Chat/Flow simulation completed with Strawberry Sundae. |
| `.staging-evidence/2026-11-24-staff-access/` | Four restricted staff UI views; no API status assertions. |
| `.staging-evidence/2026-11-24-store-hours-final/` | Explicit hours edit/removal/detail-only preservation and outer viewport fit. |
| `.staging-evidence/2026-11-24T01-56-35.334Z/` | 66 early captures; provider-loading text flagged, no recorded request errors or overflow; older harness temporarily revealed navigation. |
| `.staging-evidence/2026-11-24-connectivity-check.json` | HTTP health 200, healthy setup checklist, latest recorded inbound/signature timestamps and initial native order completed. |

Producer scripts reviewed: all `scripts/staging-*.mjs`. `staging-ui-check.mjs` captures routes/panels and reports network/console/page/overflow signals; it is not a semantic or visual guarantee for every interaction. The CRUD/every-store/hours scripts operate forms and assert persisted state; knowledge uses Admin AI test chat; mode scripts separate unbound publication, provider publication and activation; order/cancellation scripts assert exact references and operations/stock; email only records request/send responses; operations records handoff state and staff actions. `staging-runtime-state.mjs` saves a private summary, not message history. `staging-create-qa-org.mjs`, second-org/catalogue/readiness/channel scripts establish the separate workspace and its setup. Debug/recovery scripts explain why an initial failure need not establish a product defect. No staging script was run while preparing this document.
