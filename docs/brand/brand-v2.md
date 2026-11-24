# ZidiCommerce brand v2

## Concept 01 — Conversation tail (recommended)

A broad Z ends in a single downward conversation tail: selling starts in a conversation and resolves into an order. The tail belongs to the letter itself, rather than adding a familiar circular messenger badge. Ten-unit horizontal bands and a generous diagonal keep the silhouette readable at 16px; the tail extends eight units below the baseline as a deliberate optical counterweight. Charcoal on light and sunflower yellow on dark give the same mark two restrained expressions. The hand-drawn Zidi lettering combines square dots with a softly curved d, while the smaller geometric Commerce descriptor establishes product hierarchy.

## Concept 02 — Receipt cut

A compact Z carries a transparent receipt-shaped cutout in its upper terminal. The rectangular counter and two notches suggest a completed transaction without a bag handle, shopping-cart pictogram or vendor initial inside another company's bag silhouette. The receipt is a secondary discovery at larger sizes; at 16px the Z remains the primary reading. This is the most explicitly transactional concept, suited to receipts, settlement and order management, though less expressive of conversational selling than Concept 01. All holes are true even-odd negative space, so the asset works on arbitrary flat backgrounds.

## Concept 03 — Orchestrated layers

Three separate bands form a Z: a clipped upper shelf, a diagonal connector and a grounded lower shelf. Four-unit air gaps keep the layers distinct while the common 40-unit envelope makes them read as one system. The bands suggest catalogue, conversation and fulfilment coordinated through the platform; they deliberately do not assign separate brand colours to the four selling modes. Its open construction feels architectural and scales well, although the separated diagonal is a more abstract Z than the first two proposals.

## Recommendation and usage

Adopt Concept 01: its single, memorable silhouette connects the Zidi name to conversational commerce without relying on a generic storefront. Concept 02 is the alternative if transaction management becomes the central brand story; Concept 03 is strongest for a broader orchestration platform. These are original hand-authored explorations, not a trademark clearance claim.

Use the mark alone below a 160px lockup width; the descriptor is for larger placement. Keep at least 12 viewBox units of clear space outside the mark's visible bounds and half the Zidi cap height around the lockup. Never stretch, rotate, add shadows or place the light mark on dark. Use the yellow dark asset on #171715 or #20201E. Lockups are supplied in charcoal; the preview also demonstrates a warm-white reversed lockup. Production should derive its reverse colour from theme tokens. No tile is required; a yellow tile with charcoal geometry can be reserved for app icons. No SVG contains fonts, text elements, raster data, masks or external dependencies. Lettering consists of manually drawn filled paths and uniform 1.8-unit descriptor strokes.

## Implementation checklist — later adoption, outside this docs-only task

- [ ] Approve the concept; retain these three source explorations as review records. Check the chosen silhouette against relevant trademark registers before commercial registration.
- [ ] Replace symbol paths and the lockup in `apps/admin/src/components/brand/ZidiCommerceLogo.tsx`; preserve `compact`, `tone`, `subtitle`, class hooks and accessible names. Use decorative SVG semantics when adjacent accessible product text already names the brand. Avoid duplicate title IDs in repeated React instances.
- [ ] Replace `apps/admin/public/zidi-commerce-symbol.svg` and `zidi-commerce-symbol-dark.svg`; update `apps/admin/index.html` icon links for light/dark preferences. Generate and inspect 16/32px ICO, 180px touch icon and any manifest sizes; retain safe padding and verify at 1× display scale.
- [ ] Create `apps/field/public/` (currently absent), install corresponding favicon assets, and add icon links to `apps/field/index.html`. Confirm field product naming separately: its current title is “Lagos Home Services”.
- [ ] Apply `palette.md` to `packages/shared/src/zidi-tokens.css`; both apps import it. Keep violet focus and existing status foregrounds. Add explicit brand, preview and essential-boundary semantics.
- [ ] Reconcile admin aliases in `apps/admin/src/styles/global.css`: retain shared mappings, replace hardcoded muted/surface/border/status aliases, preserve `--zidi-focus`, and audit later overrides. Map `--accent` and `--accent-strong` to `--z-focus`. Map status aliases directly to their `--z-*` foregrounds.
- [ ] Audit field CSS fallbacks and white control fills before claiming dark-theme support. Token replacement alone does not complete theme migration.
- [ ] Apply the same identity to Automated Commerce, Guided Questions, WhatsApp Flow and AI Conversation; preserve understandable mode labels and existing workflows.
- [ ] Update the Zidi design skill only in a separate authorized task: section 3 records this original identity and reference boundaries; section 5 specifies palette roles, geometric logo rules and semantic tokens; section 12 adds the documented contrast pairings, visible focus and non-colour status cues; section 16 requires favicon, small-size, light/dark and responsive visual review. Do not edit the external skill as part of this deliverable.
- [ ] Check sidebar collapsed/expanded, login, mobile navigation, receipts and previews. Validate keyboard focus, real payment statuses and all four conversational modes. Test actual adjacent colours, not just reference swatches.

## Verification

Contrast ratios in `palette.md` and `brand-preview.html` are computed from the same exact hex values with WCAG relative luminance, with assertions for all intended text and essential-graphic pairs. Every SVG is checked with `xmllint --noout`. The HTML is self-contained with inline CSS and SVG geometry; no external URLs, scripts, font requests or image requests. The preview contains static component specimens rather than live application controls.

Browser visual verification could not run in this session: the browser runtime failed to initialize and the fallback browser host was unavailable. XML, contrast and external-resource checks passed; inspect the static preview visually before adoption.
