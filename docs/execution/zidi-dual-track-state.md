# Zidi staging execution

Branch: codex/controlled-beta-release-20261111. Baseline: c57e89c.

## Staging deployments
Railway staging API 38ccbbca-600f-4e95-94ec-570855d4aa4e is online at checkpoint 411c8a1. Admin baseline e2613463-09e5-4cb3-9457-4ac26add38d7 is online. Database health OK.

## Contracts and decisions
Preserve journey and version schemas, tenant scope, immutable published versions, existing permissions and audits. Studio edits draft graphs through existing journey APIs. New required APIs and migrations must be documented before integration. Only designated staging WhatsApp recipient and Paystack test payments.

## Subagent A progress
Checkpoint 1: optional definition.entry_cta and steps[].position persist across draft/publish; GET detail.channel_default exposes active scoped journey_id, version_id and mode. Migration 000035 adds published entry labels to bindings. Legacy menu no longer overrides a selected default for fresh greetings. Flow adapter forwards configured CTA. Duplicate trigger publication returns a merchant-facing error. Bot, runtime, WhatsApp package suites pass locally including default precedence and metadata round-trip regression. Lead review pending. Remaining: live staging inventory, model, native Flow deployment, real-device journeys.

## Subagent B progress
Studio checkpoint: existing versioned saves, validate, publish, activation, field inspector, actual preview sessions and native deployment controls are preserved. Added pannable, zoomable graph with labeled option/failure routes, draggable snapped nodes, fit and automatic layout, mobile canvas and active default/CTA settings. Contracts agreed with A: optional steps[].position, entry_cta and detail.channel_default. Admin typecheck/build passed. Browser testing pending. Lead owns auth and setup. Template library and dashboard next.

## Integration, blockers, tests and remaining work
Lead owns integration and staging verification. Login and read-only baseline dashboard, catalogue, inventory, stores and journey list checked through Chrome at 1440, 1024 and 390px. Inventory displayed 232 rows at each width. Go baseline suite passed. PostgreSQL migration and single-use recovery tests passed. Auth UI and setup changes committed. Runtime checkpoint reviewed and deployed. Confirm four live journeys, payment and fulfilment, studio lifecycle, three viewport sizes and full automated suites.
