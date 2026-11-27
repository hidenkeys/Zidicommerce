# ZidiCommerce palette v3

Applied in `packages/shared/src/zidi-tokens.css` (the source of truth). The palette follows zidihq.com: sunflower yellow `#FFC51A`, charcoal `#17191A` and white on an off-white `#F4F6F1` canvas, with deep green for section labels and success, and blue for focus, links and data. Violet is retired.

Rules:

- Yellow is a fill behind charcoal text (primary buttons, the active navigation pill, step markers). It is never text or an essential boundary on white.
- Charcoal carries the navigation sidebar, the setup hero and dark secondary actions.
- Blue `--z-focus` is the only focus and link colour. Selection uses the yellow tint `--z-selected`.
- Form fields use `--z-control-border` (3:1 boundary). Buttons use the lighter `--z-button-border` because their label identifies the control.

Ratios use WCAG sRGB relative luminance. AA normal text needs 4.5:1, large text and essential non-text boundaries need 3:1. This file is generated from the token values; regenerate it whenever they change.

## Semantic tokens

| Variable | Role | Light | Dark | Light / white | Dark / dark canvas |
|---|---|---|---|---|---|
| `--z-brand` | Brand fill (never text on white) | #FFC51A | #FFC51A | 1.58:1 | 11.67:1 |
| `--z-brand-strong` | Brand edge / selected row marker | #E0A800 | #FFD54A | 2.15:1 | 13.09:1 |
| `--z-brand-tint` | Brand tint surface | #FFF6D6 | #3A3115 | 1.08:1 | 1.44:1 |
| `--z-on-brand` | Text on brand | #17191A | #17191A | 17.64:1 | 1.05:1 |
| `--z-canvas` | App canvas | #F4F6F1 | #121414 | 1.09:1 | 1.00:1 |
| `--z-surface` | Cards and panels | #FFFFFF | #17191A | 1.00:1 | 1.05:1 |
| `--z-surface-subtle` | Table headers, subtle fills | #F8F9F6 | #202422 | 1.06:1 | 1.18:1 |
| `--z-ink` | Body text | #17191A | #F4F6F1 | 17.64:1 | 16.99:1 |
| `--z-muted` | Secondary text | #5B605C | #B8BDB9 | 6.42:1 | 9.70:1 |
| `--z-line` | Decorative divider | #E4E7E1 | #2F3431 | 1.25:1 | 1.46:1 |
| `--z-line-soft` | Row divider | #EEF0EB | #2A2E2C | 1.15:1 | 1.34:1 |
| `--z-control-border` | Form field boundary (3:1) | #8A8F8B | #7C827E | 3.29:1 | 4.71:1 |
| `--z-button-border` | Button outline (text carries the control) | #D5D9D2 | #3A3F3B | 1.43:1 | 1.72:1 |
| `--z-primary` | Primary action fill | #FFC51A | #FFC51A | 1.58:1 | 11.67:1 |
| `--z-primary-hover` | Primary hover | #F5B800 | #FFD54A | 1.79:1 | 13.09:1 |
| `--z-on-primary` | Primary action text | #17191A | #17191A | 17.64:1 | 1.05:1 |
| `--z-primary-strong` | Dark secondary action | #17191A | #F4F6F1 | 17.64:1 | 16.99:1 |
| `--z-on-primary-strong` | Text on dark action | #FFFFFF | #17191A | 1.00:1 | 1.05:1 |
| `--z-focus` | Focus ring, links, data | #3059D8 | #8EA9FF | 5.92:1 | 8.16:1 |
| `--z-selected` | Selected surface | #FFF6D6 | #3A3115 | 1.08:1 | 1.44:1 |
| `--z-accent-green` | Section eyebrows, assistant card | #10634C | #7FD1AE | 7.21:1 | 10.24:1 |
| `--z-danger` | Danger text | #9D2831 | #FFB5BC | 7.55:1 | 11.08:1 |
| `--z-danger-bg` | Danger surface | #FFF2F2 | #41262D | 1.09:1 | 1.36:1 |
| `--z-success` | Success text | #10634C | #88DAB0 | 7.21:1 | 11.18:1 |
| `--z-success-bg` | Success surface | #E8F4EE | #1C3329 | 1.13:1 | 1.37:1 |
| `--z-warning` | Warning text | #6B4E00 | #FFD970 | 7.74:1 | 13.57:1 |
| `--z-warning-bg` | Warning surface | #FFF6D6 | #3A3115 | 1.08:1 | 1.44:1 |
| `--z-info` | Info text | #3059D8 | #A9BEFF | 5.92:1 | 10.12:1 |
| `--z-info-bg` | Info surface | #EDF1FF | #222C4A | 1.13:1 | 1.34:1 |
| `--z-wa-wallpaper` | Preview wallpaper | #EAF0EB | #1B211F | 1.16:1 | 1.13:1 |
| `--z-wa-outgoing` | Preview outgoing | #FFE89A | #4A3F17 | 1.22:1 | 1.77:1 |
| `--z-wa-incoming` | Preview incoming | #FFFFFF | #2A302D | 1.00:1 | 1.37:1 |
| `--z-wa-tick` | Read receipt | #3059D8 | #8EA9FF | 5.92:1 | 8.16:1 |
| `--z-wa-header` | Preview header | #10634C | #173C32 | 7.21:1 | 1.52:1 |
| `--z-wa-on-header` | Preview header text | #FFFFFF | #F4F6F1 | 1.00:1 | 16.99:1 |
| `--z-wa-ink` | Preview message text | #17191A | #F4F6F1 | 17.64:1 | 16.99:1 |

## Sidebar (charcoal in both themes)

| Variable | Role | Value | On `--z-sidebar-bg` |
|---|---|---|---|
| `--z-sidebar-bg` | Panel | #17191A | 1.00:1 |
| `--z-sidebar-card` | Raised card | #202422 | 1.12:1 |
| `--z-sidebar-line` | Divider | #2A2E2C | 1.28:1 |
| `--z-sidebar-track` | Progress track | #2F3431 | 1.39:1 |
| `--z-sidebar-ink` | Navigation text | #C9CDC9 | 10.97:1 |
| `--z-sidebar-muted` | Group labels, hints | #8E938F | 5.65:1 |
| `--z-sidebar-strong` | Active / hover text | #FFFFFF | 17.64:1 |

## Neutral scale

| Variable | Hex | On white | On #121414 |
|---|---|---|---|
| `--z-neutral-50` | #F8F9F6 | 1.06:1 | 17.50:1 |
| `--z-neutral-100` | #F1F2EF | 1.12:1 | 16.45:1 |
| `--z-neutral-200` | #E4E7E1 | 1.25:1 | 14.80:1 |
| `--z-neutral-300` | #D5D9D2 | 1.43:1 | 12.93:1 |
| `--z-neutral-400` | #A4A8A3 | 2.41:1 | 7.67:1 |
| `--z-neutral-500` | #737873 | 4.50:1 | 4.11:1 |
| `--z-neutral-600` | #5B605C | 6.42:1 | 2.88:1 |
| `--z-neutral-700` | #3A3F3B | 10.75:1 | 1.72:1 |
| `--z-neutral-800` | #202422 | 15.71:1 | 1.18:1 |
| `--z-neutral-900` | #17191A | 17.64:1 | 1.05:1 |
| `--z-neutral-950` | #121414 | 18.49:1 | 1.00:1 |

## Required pairing checks

| Pair | Light | Dark | Use |
|---|---|---|---|
| `--z-ink` / `--z-surface` | 17.64:1 | 16.21:1 | Body text |
| `--z-ink` / `--z-canvas` | 16.21:1 | 16.99:1 | Body text on canvas |
| `--z-muted` / `--z-surface` | 6.42:1 | 9.26:1 | Secondary text |
| `--z-muted` / `--z-canvas` | 5.90:1 | 9.70:1 | Secondary text on canvas |
| `--z-on-primary` / `--z-primary` | 11.13:1 | 11.13:1 | Primary button label |
| `--z-on-primary` / `--z-primary-hover` | 9.86:1 | 12.49:1 | Primary button hover |
| `--z-on-brand` / `--z-brand` | 11.13:1 | 11.13:1 | Active nav pill, step markers |
| `--z-on-primary-strong` / `--z-primary-strong` | 17.64:1 | 16.21:1 | Dark action label |
| `--z-focus` / `--z-surface` | 5.92:1 | 7.79:1 | Links and focus ring |
| `--z-ink` / `--z-selected` | 16.30:1 | 11.84:1 | Selected row text |
| `--z-control-border` / `--z-surface` | 3.29:1 | 4.50:1 | Field boundary (3:1) |
| `--z-accent-green` / `--z-surface` | 7.21:1 | 9.77:1 | Eyebrow text |
| `--z-accent-green` / `--z-canvas` | 6.63:1 | 10.24:1 | Eyebrow text on canvas |
| `--z-danger` / `--z-danger-bg` | 6.92:1 | 8.17:1 | Danger pill |
| `--z-success` / `--z-success-bg` | 6.39:1 | 8.17:1 | Success pill |
| `--z-warning` / `--z-warning-bg` | 7.15:1 | 9.45:1 | Warning pill |
| `--z-info` / `--z-info-bg` | 5.25:1 | 7.53:1 | Info pill |
| `--z-wa-ink` / `--z-wa-outgoing` | 14.50:1 | 9.58:1 | Outgoing message |
| `--z-wa-ink` / `--z-wa-incoming` | 17.64:1 | 12.38:1 | Incoming message |
| `--z-wa-on-header` / `--z-wa-header` | 7.21:1 | 11.16:1 | Preview header |
| `--z-sidebar-ink` / `--z-sidebar-bg` | 10.97:1 | n/a | Navigation text |
| `--z-sidebar-muted` / `--z-sidebar-bg` | 5.65:1 | n/a | Group labels |
| `--z-sidebar-strong` / `--z-sidebar-card` | 15.71:1 | n/a | Workspace card text |
| `--z-brand` / `--z-sidebar-bg` | 11.13:1 | n/a | Yellow on charcoal (hero eyebrow, counts) |
| `--z-surface` / `--z-accent-green` | 7.21:1 | n/a | Assistant card text |

Status backgrounds are subtle fills, not boundaries: always pair them with their foreground colour and a text label. `--z-line` and `--z-line-soft` are deliberately below 3:1 and must not be the only boundary of an input.

The WhatsApp preview colours are simulation colours (yellow outgoing bubbles, a green header), not official WhatsApp brand specifications. Keep the visible "WhatsApp preview" label.

## Where the theme lives

- Tokens: `packages/shared/src/zidi-tokens.css` (both apps and the shared auth layout).
- Admin component layer: `apps/admin/src/styles/zidi-brand.css`, loaded after `global.css`. `global.css` holds no hard-coded hex; its legacy aliases (`--ink`, `--line`, `--accent`, …) resolve to these tokens.
- Auth screens: `packages/shared/src/access.css` and `apps/admin/src/components/auth-forms.css`.
- Field portals: `apps/field/src/styles.css` (brand layer at the end of the file).
