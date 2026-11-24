# ZidiCommerce palette v2

Proposal only; application files remain unchanged. Warm mineral neutrals, sunflower yellow and charcoal preserve recognition without making the workspace yellow. Keep violet: the existing focus hue is distinct from brand yellow, success green and information blue. Use it only for links, focus and selection, not decorative gradients. Yellow is a brand fill or highlight, never small text on white. Primary actions remain charcoal in light mode and warm white in dark mode.

Contrast uses WCAG sRGB relative luminance (linearize at 0.04045; weights 0.2126/0.7152/0.0722), rounded only for display. AA normal text requires 4.5:1, large text 3:1; essential non-text boundaries require 3:1. Logos are exempt, but small-size clarity still matters. Dark reference = #171715; actual dark surface = #20201E.

## Semantic tokens

Ratios below compare each light value with white and each dark value with dark canvas; surface fills and dividers are not text colours. Pair-specific checks follow.

| Variable | Role | Light | Dark | Light / white | Dark / dark canvas |
|---|---|---|---|---|---|
| `--z-brand` | Brand accent | #F2C438 | #F2C438 | 1.65:1 | 10.88:1 |
| `--z-on-brand` | On brand | #20201E | #20201E | 16.32:1 | 1.10:1 |
| `--z-canvas` | Canvas | #FAFAF8 | #171715 | 1.05:1 | 1.00:1 |
| `--z-surface` | Surface | #FFFFFF | #20201E | 1.00:1 | 1.10:1 |
| `--z-ink` | Ink | #20201E | #FAFAF8 | 16.32:1 | 17.18:1 |
| `--z-muted` | Secondary text | #606057 | #CDCDC4 | 6.35:1 | 11.22:1 |
| `--z-line` | Decorative divider | #E5E5DF | #484841 | 1.26:1 | 1.95:1 |
| `--z-control-border` | Essential control boundary | #77776D | #A4A49A | 4.52:1 | 7.14:1 |
| `--z-primary` | Primary action | #20201E | #FAFAF8 | 16.32:1 | 17.18:1 |
| `--z-on-primary` | Primary text | #FFFFFF | #20201E | 1.00:1 | 1.10:1 |
| `--z-focus` | Interaction / focus | #6745BB | #BBA3FF | 6.68:1 | 8.37:1 |
| `--z-selected` | Selected surface | #F1ECFC | #34294A | 1.16:1 | 1.34:1 |
| `--z-danger` | Danger text | #9D2831 | #FFB5BC | 7.55:1 | 10.76:1 |
| `--z-danger-bg` | Danger surface | #FFF2F2 | #41262D | 1.09:1 | 1.32:1 |
| `--z-success` | Success text | #206344 | #88DAB0 | 7.17:1 | 10.86:1 |
| `--z-success-bg` | Success surface | #EAF4EE | #20382B | 1.12:1 | 1.42:1 |
| `--z-warning` | Warning text | #805311 | #F4CD85 | 6.64:1 | 11.90:1 |
| `--z-warning-bg` | Warning surface | #FFF4DA | #3C301C | 1.09:1 | 1.39:1 |
| `--z-info` | Info text | #2D527C | #A5CAFF | 8.05:1 | 10.68:1 |
| `--z-info-bg` | Info surface | #EDF3FA | #243347 | 1.12:1 | 1.40:1 |
| `--z-wa-wallpaper` | Preview wallpaper | #EEECE5 | #1B211F | 1.18:1 | 1.10:1 |
| `--z-wa-outgoing` | Preview outgoing | #F5E7B1 | #443D25 | 1.24:1 | 1.66:1 |
| `--z-wa-incoming` | Preview incoming | #FFFFFF | #2A302D | 1.00:1 | 1.33:1 |
| `--z-wa-tick` | Read receipt blue | #246A91 | #86C6E8 | 5.92:1 | 9.62:1 |
| `--z-wa-header` | Preview header | #303B35 | #252F29 | 11.65:1 | 1.30:1 |
| `--z-wa-on-header` | Preview header text | #FFFFFF | #FAFAF8 | 1.00:1 | 17.18:1 |
| `--z-wa-ink` | Preview message text | #20201E | #FAFAF8 | 16.32:1 | 17.18:1 |

## Neutral scale

Fixed primitives in both themes; semantic roles reverse by theme.

| Variable | Hex | On white | On #171715 |
|---|---|---|---|
| `--z-neutral-50` | #FAFAF8 | 1.05:1 | 17.18:1 |
| `--z-neutral-100` | #F3F3EF | 1.11:1 | 16.14:1 |
| `--z-neutral-200` | #E5E5DF | 1.26:1 | 14.19:1 |
| `--z-neutral-300` | #CDCDC4 | 1.60:1 | 11.22:1 |
| `--z-neutral-400` | #A4A49A | 2.51:1 | 7.14:1 |
| `--z-neutral-500` | #77776D | 4.52:1 | 3.97:1 |
| `--z-neutral-600` | #606057 | 6.35:1 | 2.83:1 |
| `--z-neutral-700` | #484841 | 9.22:1 | 1.95:1 |
| `--z-neutral-800` | #33332E | 12.70:1 | 1.41:1 |
| `--z-neutral-900` | #20201E | 16.32:1 | 1.10:1 |
| `--z-neutral-950` | #171715 | 17.95:1 | 1.00:1 |

## Required pairing checks

| Pair | Light | Dark | Use |
|---|---|---|---|
| `--z-ink` / `--z-surface` | 16.32:1 | 15.62:1 | Body text; pass |
| `--z-muted` / `--z-surface` | 6.35:1 | 10.20:1 | Secondary text; pass |
| `--z-on-primary` / `--z-primary` | 16.32:1 | 15.62:1 | Button text; pass |
| `--z-on-brand` / `--z-brand` | 9.89:1 | 9.89:1 | Brand text; pass |
| `--z-focus` / `--z-surface` | 6.68:1 | 7.61:1 | Link text and focus ring; pass |
| `--z-focus` / `--z-selected` | 5.77:1 | 6.27:1 | Selected text; pass |
| `--z-control-border` / `--z-surface` | 4.52:1 | 6.49:1 | Essential boundary (3:1); pass |
| `--z-danger` / `--z-danger-bg` | 6.92:1 | 8.17:1 | Danger badge; pass |
| `--z-success` / `--z-success-bg` | 6.37:1 | 7.64:1 | Success badge; pass |
| `--z-warning` / `--z-warning-bg` | 6.07:1 | 8.53:1 | Warning badge; pass |
| `--z-info` / `--z-info-bg` | 7.20:1 | 7.62:1 | Info badge; pass |
| `--z-wa-ink` / `--z-wa-outgoing` | 13.17:1 | 10.36:1 | Outgoing message; pass |
| `--z-wa-ink` / `--z-wa-incoming` | 16.32:1 | 12.89:1 | Incoming message; pass |
| `--z-wa-tick` / `--z-wa-outgoing` | 4.78:1 | 5.80:1 | Receipt icon (3:1); pass |
| `--z-wa-on-header` / `--z-wa-header` | 11.65:1 | 13.24:1 | Preview header; pass |

All four status foregrounds meet AA normal text on white in light mode and on both #171715 and #20201E in dark mode. Light status hues must not be reused on dark, nor dark hues on white. Status backgrounds are subtle fills, not accessible outlines: add the matching status foreground for a required boundary and a text label or icon for meaning. Decorative `--z-line` is intentionally below 3:1; use `--z-control-border` for an essential input boundary. Focus is a 2px solid ring with 2px surface-coloured separation so it never touches a similarly coloured control.

WhatsApp previews use warm yellow outgoing bubbles, mineral wallpaper and a charcoal-green header, rather than copying the provider's green identity. Keep a visible “WhatsApp preview” label; these are simulation colours, not official WhatsApp brand specifications. Use `--z-wa-ink` for all message text; ticks convey read state alongside an accessible “Read” label. The blue is a receipt indicator, not a second interaction colour. In the product, the four selling modes share this brand: Automated Commerce, Guided Questions, WhatsApp Flow and AI Conversation use text labels, not four competing accent palettes.

## Migration map

Source of truth: `packages/shared/src/zidi-tokens.css`; admin aliases in `apps/admin/src/styles/global.css` must continue to resolve to shared semantics.

Existing tokens that change (old light / dark → new light / dark):

- `--z-canvas`: #f7f8fa / #17181d → #FAFAF8 / #171715.
- `--z-surface`: #fff / #1d1f26 → #FFFFFF / #20201E.
- `--z-ink`: #17191f / #f5f4f8 → #20201E / #FAFAF8.
- `--z-muted`: #595f6c / #bdbecc → #606057 / #CDCDC4.
- `--z-line`: #dedfe5 / #44454f → #E5E5DF / #484841.
- `--z-primary`: #17191f / #f5f4f8 → #20201E / #FAFAF8.
- `--z-on-primary`: #fff / #17191f → #FFFFFF / #20201E.

Keep existing `--z-focus`, `--z-selected`, all four status foregrounds and `--z-danger-bg` unchanged. Add `--z-brand`, `--z-on-brand`, `--z-control-border`, the neutral primitives, success/warning/info backgrounds and all `--z-wa-*` tokens above. Replace the logo's hardcoded #ffcc22 with #F2C438 on adoption; #231f20 becomes #20201E. No spacing, radius, font or motion tokens change.

Map admin `--canvas` → `--z-canvas`, `--surface` → `--z-surface`, `--ink` → `--z-ink`, `--ink-subtle` and `--ink-muted` → `--z-muted`, `--line` → `--z-line`, `--line-strong` → `--z-control-border`, `--surface-subtle` → light neutral-100 / dark neutral-800. Keep `--zidi-focus`, `--accent` and `--accent-strong` → `--z-focus`. Map `--success`, `--warning`, `--danger` and `--info` to the matching shared `--z-*` foregrounds. Audit later local CSS overrides and hardcoded colours before migration; this proposal does not apply replacements.
