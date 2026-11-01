# Dashboard v3 — the tokens

Part of the design of the dashboard. The main file is
[../design-dashboard.md](../design-dashboard.md); this file holds §3 of it.

The tokens live in `dashboard/web/css/tokens.css`. The names are ours; the
values are the ones of the reference web UI (vllm-rdna-dash by CrispyGinger,
Apache-2.0), moved where a pair failed AA.

Every pair below was measured with the WCAG 2.1 relative luminance formula;
the numbers are the ratios, not a target. Text needs 4.5:1, the edge of a
control and the large text need 3:1.

## The fonts

The three families of the reference are self-hosted: the Latin subset of each
file, no CDN, `font-display: swap`, the system stack as the fallback.

| Token | Family | File | Licence |
|---|---|---|---|
| `--font-display` | Montserrat (upright and italic) | `web/fonts/montserrat-latin.woff2`, `web/fonts/montserrat-italic-latin.woff2` | SIL OFL 1.1 |
| `--font-body` | Inter | `web/fonts/inter-latin.woff2` | SIL OFL 1.1 |
| `--font-mono` | JetBrains Mono | `web/fonts/jetbrains-mono-latin.woff2` | SIL OFL 1.1 |

The four files are 128 KB together. The licence texts are in `web/fonts/`.

## Dark palette (the default)

| Token | Hex | Role |
|---|---|---|
| `--bg` | `#0e0e10` | the page |
| `--surface` | `#141417` | the cards |
| `--surface-2` | `#1b1b1f` | the wells, the pills, the banners |
| `--surface-3` | `#26262c` | the hover, one step above the third surface |
| `--border` | `#2a2a30` | the hairline between the rows (decorative) |
| `--border-strong` | `#66666b` | the edge of a control (3:1) |
| `--text` | `#f2f1ee` | the text |
| `--text-2` | `#c3c2b7` | the second level of text |
| `--muted` | `#8b8a84` | the labels, the units, the captions |
| `--accent` | `#ff3b30` | the identity: the active tab, the links, the accent shadow |
| `--on-accent` | `#0b0b0b` | the text on the accent |
| `--key` | `#ffd21f` | the one number that matters on a screen |
| `--primary-bg` `--primary-fg` | `#ffd21f` `#0b0b0b` | the one filled button of a tab |
| `--ok` `--warn` `--serious` `--fail` `--skip` `--info` | `#0ca30c` `#fab219` `#ec835a` `#d85a5a` `#8b8a84` `#3987e5` | the four-step status scale and the two quiet ones |
| `--series-1..6` | `#3987e5` `#d95926` `#199e70` `#c98500` `#9ec5f4` `#6da7ec` | the categorical colours of the charts |
| `--seq-1..6` | `#1a1a19` `#184f95` `#256abf` `#3987e5` `#6da7ec` `#9ec5f4` | the sequential ramp of the matrix |

| Pair | Ratio | Need |
|---|---|---|
| text on bg | 17.07 | 4.5 |
| text on surface | 16.28 | 4.5 |
| text on surface-2 | 15.20 | 4.5 |
| text-2 on surface | 10.26 | 4.5 |
| text-2 on surface-2 | 9.58 | 4.5 |
| muted on bg | 5.57 | 4.5 |
| muted on surface | 5.31 | 4.5 |
| muted on surface-2 | 4.96 | 4.5 |
| accent on surface | 5.18 | 4.5 |
| accent on surface-2 | 4.84 | 4.5 |
| on-accent on accent | 5.55 | 4.5 |
| key on surface | 12.69 | 4.5 |
| key on surface-2 | 11.85 | 4.5 |
| ok on surface | 5.48 | 4.5 |
| warn on surface | 10.02 | 4.5 |
| serious on surface | 6.97 | 4.5 |
| fail on surface | 4.84 | 4.5 |
| fail on surface-2 | 4.52 | 4.5 |
| skip on surface | 5.31 | 4.5 |
| info on surface | 5.05 | 4.5 |
| border-strong on bg | 3.38 | 3.0 |
| border-strong on surface | 3.22 | 3.0 |
| border-strong on surface-2 | 3.01 | 3.0 |
| series-1 on surface | 5.05 | 3.0 |
| series-2 on surface | 4.73 | 3.0 |
| series-3 on surface | 5.40 | 3.0 |
| series-4 on surface | 5.99 | 3.0 |

## Light palette (the override)

| Token | Hex | Role |
|---|---|---|
| `--bg` | `#fafaf8` | the page |
| `--surface` | `#f1f0ec` | the cards |
| `--surface-2` | `#e8e7e2` | the wells, the pills, the banners |
| `--surface-3` | `#dedcd6` | the hover |
| `--border` | `#dddcd6` | the hairline |
| `--border-strong` | `#858480` | the edge of a control (3:1) |
| `--text` | `#0b0b0b` | the text |
| `--text-2` | `#3d3c39` | the second level of text |
| `--muted` | `#696863` | the labels, the units, the captions |
| `--accent` | `#ca2118` | the identity |
| `--on-accent` | `#ffffff` | the text on the accent |
| `--key` | `#846300` | the one number that matters on a screen |
| `--primary-bg` `--primary-fg` | `#ca2118` `#ffffff` | the one filled button of a tab |
| `--ok` `--warn` `--serious` `--fail` `--skip` `--info` | `#097909` `#88610e` `#9a563b` `#bd3636` `#696863` `#2568ba` | the status scale |
| `--series-1..6` | `#2a78d6` `#d86030` `#179669` `#b37900` `#5598e7` `#256abf` | the categorical colours |
| `--seq-1..6` | `#f0efec` `#cde2fb` `#9ec5f4` `#5598e7` `#256abf` `#104281` | the sequential ramp |

| Pair | Ratio | Need |
|---|---|---|
| text on bg | 18.83 | 4.5 |
| text on surface | 17.26 | 4.5 |
| text on surface-2 | 15.90 | 4.5 |
| text-2 on surface | 9.67 | 4.5 |
| muted on surface | 4.90 | 4.5 |
| muted on surface-2 | 4.51 | 4.5 |
| accent on surface | 4.92 | 4.5 |
| accent on surface-2 | 4.53 | 4.5 |
| on-accent on accent | 5.61 | 4.5 |
| key on surface | 4.89 | 4.5 |
| key on surface-2 | 4.50 | 4.5 |
| ok on surface | 4.91 | 4.5 |
| warn on surface | 4.89 | 4.5 |
| serious on surface | 4.89 | 4.5 |
| fail on surface | 4.92 | 4.5 |
| skip on surface | 4.90 | 4.5 |
| info on surface | 4.89 | 4.5 |
| border-strong on bg | 3.58 | 3.0 |
| border-strong on surface | 3.28 | 3.0 |
| border-strong on surface-2 | 3.02 | 3.0 |
| series-1 on surface | 3.87 | 3.0 |
| series-2 on surface | 3.27 | 3.0 |
| series-3 on surface | 3.29 | 3.0 |
| series-4 on surface | 3.26 | 3.0 |

## The values that were moved

The reference palette is dark first and light second, and a few of its pairs
do not reach AA on a light background. Each value below was moved by the
smallest step that passes; the original value is the one of the reference.

| Token | Palette | Reference | Here | Why |
|---|---|---|---|---|
| `--fail` | dark | `#d03b3b` (3.83 on the surface) | `#d85a5a` (4.84) | the text of a failure needs 4.5:1 |
| `--border-strong` | dark | `#3a3a41` (1.63 on the surface) | `#66666b` (3.22) | the edge of a control needs 3:1 |
| `--muted` | light | `#6b6a65` (4.38 on surface-2) | `#696863` (4.51) | the text needs 4.5:1 |
| `--accent` | light | `#e8261c` (3.90 on the surface) | `#ca2118` (4.92) | the text needs 4.5:1 |
| `--key` | light | `#b88a00` (2.76 on the surface) | `#846300` (4.89) | the key number is text |
| `--ok` | light | `#0ca30c` (2.94 on the surface) | `#097909` (4.91) | the text needs 4.5:1 |
| `--warn` | light | `#fab219` (1.61 on the surface) | `#88610e` (4.89) | the text needs 4.5:1 |
| `--serious` | light | `#ec835a` (2.31 on the surface) | `#9a563b` (4.89) | the text needs 4.5:1 |
| `--fail` | light | `#d03b3b` (4.21 on the surface) | `#bd3636` (4.92) | the text needs 4.5:1 |
| `--info` | light | `#2a78d6` (3.87 on the surface) | `#2568ba` (4.89) | the text needs 4.5:1 |
| `--series-2` | light | `#eb6834` (2.81 on the surface) | `#d86030` (3.27) | a chart colour needs 3:1 |
| `--series-3` | light | `#1baf7a` (2.47 on the surface) | `#179669` (3.29) | a chart colour needs 3:1 |
| `--series-4` | light | `#eda100` (1.90 on the surface) | `#b37900` (3.26) | a chart colour needs 3:1 |
| `--border-strong` | light | `#c9c8c1` (1.47 on the surface) | `#858480` (3.28) | the edge of a control needs 3:1 |

Two values are ours and are recorded here: `--surface-3` (`#26262c` dark,
`#dedcd6` light) is one step above the third surface of the reference, for the
hover; `--log-bg` (`#10141a` dark, `#111418` light) is the near-black surface
of the log viewer in both palettes, so the tail reads the same in both.

The filled button of a tab is the key colour in the dark palette and the accent
in the light one: the key of the light palette is too dark for the label of a
button and too light to be text on the white page, so the button takes the
accent there. `--primary-bg` and `--primary-fg` carry that choice.

`--border` is a decorative hairline and is exempt from the 3:1 rule.
`--seq-1` and `--seq-2` are tints of a ramp and carry no contrast promise:
every cell of a matrix also carries its value as text.

## Type, spacing, radii, elevation, motion

| What | Rule |
|---|---|
| Display | `font: italic 900 clamp(38px, 6vw, 74px)/.95 var(--font-display); letter-spacing: -.025em; text-transform: uppercase;` with the hard shadow `3px 3px 0 var(--accent), 6px 6px 0 var(--shadow-ink)` (the light palette uses one layer of ink) |
| Hero number | `900 64px/1 var(--font-display)`, `--key` colour |
| Lede | 18 px, `--text-2`, `max-width: 70ch` |
| Eyebrow | 12 px 800, uppercase, `letter-spacing: .14em`, `--muted` |
| Section label | 13 px 800, uppercase, `letter-spacing: .14em`, `--muted` |
| Body | 15 px / 1.5 `--font-body` |
| Small | 13 px, `--muted` |
| Mono | 13 px `--font-mono` for the commands, the logs and the JSON |
| Tile number | `900 30px/1.1` with the unit in 13 px `--muted` |
| Spacing | 4 8 12 16 20 24 32 34 48 56 80 px |
| Radii | 6 8 9 10 16 18 999 px |
| Elevation | `--shadow-1` `0 1px 2px rgba(0,0,0,.5)` and `--shadow-2` `0 6px 20px rgba(0,0,0,.55)`; the light palette uses the ink at .08 and .12. Only the toast and the tooltip have a shadow. |
| Layout | the content `padding: 34px clamp(16px, 4vw, 56px) 80px`; a view is at most 1240 px wide; a section is a 1 px top border with 24 px of padding above it; a column grid is `repeat(auto-fit, minmax(320px, 1fr))` with a 32 px gap; a card has a 16 px radius and 20/22 px of padding |
| Motion | 120 ms on hover and focus, 200 ms on a meter, 400 ms on a progress bar, an 800 ms spin on the running pill |
| Reduced motion | `prefers-reduced-motion: reduce` sets every transition and animation to 0 ms and removes the spin |

## Chart colours

| Rule | Value |
|---|---|
| Categorical | `--series-1` … `--series-6`, in that order. A chart uses at most four series at a time. |
| Sequential | `--seq-1` … `--seq-6`, light to dark, for the matrix (the link classes of the topology). Every cell also carries its value as text. |
| The two arms of a comparison | the baseline is `--series-1`, the candidate is `--series-2`. |
| The verdict of a metric | better `--ok`, same `--skip`, worse `--fail`, noisy `--warn`. The chip always carries the word. |
| The status of a card | `--ok` `--warn` `--serious` `--fail` `--skip` `--info`, always with an icon and a word. |
| A missing value | a dashed line at the baseline of the axis and the text `–`. Never a zero. |
