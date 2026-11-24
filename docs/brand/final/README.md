# Zidi Commerce — Orchestrated layers

Concept 03 is the adopted identity. Palette v2 in [palette.md](../palette.md) is authoritative; the concept recommendation in the original exploration is historical.

## Assets

| Asset | Use |
|---|---|
| zidi-mark.svg | Charcoal mark, 64 × 64 viewBox, light backgrounds |
| zidi-mark-dark.svg | Yellow mark, 64 × 64 viewBox, dark backgrounds |
| zidi-lockup.svg | Charcoal mark and lettering, light backgrounds |
| zidi-lockup-dark.svg | Yellow mark and warm-white lettering, dark backgrounds |
| zidi-app-icon.svg | Yellow rounded square with charcoal mark |
| zidi-favicon.svg | Pixel-aligned simplified 16px tile |

All lettering is original filled path geometry, with curved geometric forms, proportional descriptor widths and 0.25-unit tracking (under 2% of its 14-unit cap height). No fonts, text elements, raster images or external references are required. Commerce sits closely below Zidi and uses a comparable optical weight.

## Usage

Keep at least 12 viewBox units around the visible mark; the 64-unit mark includes this space. Keep half the Zidi cap height around the lockup. Do not stretch, rotate, add effects or recolour individual layers. Use the reverse assets on #171715 or #20201E. Yellow is a fill/highlight, never small text on white. Primary actions use the semantic primary tokens, not yellow.

Minimum mark size: 24px; use the simplified favicon at 16px. Minimum full lockup width: 160px. For constrained placements use the mark with accessible product text. Preserve a single accessible brand name; decorative repeated symbols should be hidden from assistive technology. Subtitle content remains ordinary app text.

Both public folders contain SVG favicons, corresponding light/dark marks, SVG app tiles and 180px Apple / 192px / 512px PNG app icons. Manifests reference the PNG sizes. PNGs were generated offline with native AppKit drawing of the same tile geometry and resized with sips; sips alone could not decode SVG. No previous PNG favicon fallback existed. Field keeps its existing document title and product naming.

## Token migration

Shared tokens are the source of truth. Values below are light / dark; fixed primitives have one value. Existing violet focus, selected colours and status foregrounds remain unchanged.

| Token | Old light / dark | New light / dark |
|---|---|---|
| `--z-neutral-50` | added | #FAFAF8 |
| `--z-neutral-100` | added | #F3F3EF |
| `--z-neutral-200` | added | #E5E5DF |
| `--z-neutral-300` | added | #CDCDC4 |
| `--z-neutral-400` | added | #A4A49A |
| `--z-neutral-500` | added | #77776D |
| `--z-neutral-600` | added | #606057 |
| `--z-neutral-700` | added | #484841 |
| `--z-neutral-800` | added | #33332E |
| `--z-neutral-900` | added | #20201E |
| `--z-neutral-950` | added | #171715 |
| `--z-brand` | added | #F2C438 / #F2C438 |
| `--z-on-brand` | added | #20201E / #20201E |
| `--z-canvas` | #f7f8fa / #17181d | #FAFAF8 / #171715 |
| `--z-surface` | #fff / #1d1f26 | #FFFFFF / #20201E |
| `--z-ink` | #17191f / #f5f4f8 | #20201E / #FAFAF8 |
| `--z-muted` | #595f6c / #bdbecc | #606057 / #CDCDC4 |
| `--z-line` | #dedfe5 / #44454f | #E5E5DF / #484841 |
| `--z-control-border` | added | #77776D / #A4A49A |
| `--z-primary` | #17191f / #f5f4f8 | #20201E / #FAFAF8 |
| `--z-on-primary` | #fff / #17191f | #FFFFFF / #20201E |
| `--z-focus` | #6745bb / #bba3ff | #6745BB / #BBA3FF |
| `--z-selected` | #f1ecfc / #34294a | #F1ECFC / #34294A |
| `--z-danger` | #9d2831 / #ffb5bc | #9D2831 / #FFB5BC |
| `--z-danger-bg` | #fff2f2 / #41262d | #FFF2F2 / #41262D |
| `--z-success` | #206344 / #88dab0 | #206344 / #88DAB0 |
| `--z-success-bg` | added | #EAF4EE / #20382B |
| `--z-warning` | #805311 / #f4cd85 | #805311 / #F4CD85 |
| `--z-warning-bg` | added | #FFF4DA / #3C301C |
| `--z-info` | #2d527c / #a5caff | #2D527C / #A5CAFF |
| `--z-info-bg` | added | #EDF3FA / #243347 |
| `--z-wa-wallpaper` | added | #EEECE5 / #1B211F |
| `--z-wa-outgoing` | added | #F5E7B1 / #443D25 |
| `--z-wa-incoming` | added | #FFFFFF / #2A302D |
| `--z-wa-tick` | added | #246A91 / #86C6E8 |
| `--z-wa-header` | added | #303B35 / #252F29 |
| `--z-wa-on-header` | added | #FFFFFF / #FAFAF8 |
| `--z-wa-ink` | added | #20201E / #FAFAF8 |

Admin aliases: `--canvas`, `--surface`, `--ink`, `--line` resolve to their shared semantics; `--ink-subtle` / `--ink-muted` → `--z-muted`; `--line-strong` → `--z-control-border`; `--surface-subtle` → neutral-100 / neutral-800. `--accent`, `--accent-strong`, `--zidi-focus` → `--z-focus`. Status aliases resolve to shared foregrounds. Field uses the same shared palette; this adoption does not claim a complete Field dark-mode redesign. No spacing, radius, typography or motion tokens changed.

Legacy logo fills: `#ffcc22` → `#F2C438` (`--z-brand`); `#231f20` → `#20201E` (`--z-on-brand`).

Validation: both production builds and all three admin component tests pass; all SVGs pass xmllint and git diff --check is clean. Browser visual QA could not run: the browser tool failed to initialize and its alternate host was unavailable. The raster app icon was inspected locally.
