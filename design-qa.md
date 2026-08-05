# Design QA

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-a857d24c-f4ad-48ee-b6a9-a622096aca8e.png`,
  `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-3143c202-ecb3-4a56-a9af-cfa793eeba97.png`,
  and the supplied QuarkMux product-page reference.
- Implementation: `http://127.0.0.1:3000/`, `/models`, and `/docs`.
- Implementation screenshots: `tmp/qa/models-desktop-light-final.png`,
  `tmp/qa/models-mobile-light-final.png`, `tmp/qa/models-mobile-dark-final.png`,
  `tmp/qa/home-desktop-light-final.png`, and `tmp/qa/docs-mobile-light-final.png`.
  These local QA images are intentionally ignored by Git.
- Side-by-side comparison: `tmp/qa/models-reference-comparison-final.png`.
- Homepage copy review: `tmp/qa/home-desktop-product-copy.png` and
  `tmp/qa/home-mobile-product-copy.png`.
- Viewports: 1280 x 720 desktop and 390 x 844 mobile.
- States: models light/dark, homepage light, docs light/dark, unauthenticated public state.
- Authentication states: first-run administrator setup, registration, local login,
  configured OIDC entry, member console, and light/dark member console.

## Full-view comparison evidence

The model directory preserves the reference's compact two-column comparison pattern,
prominent search and filters, thin borders, muted secondary metadata, and highly scannable
price blocks. It intentionally uses Novro's monochrome token system, existing header and
footer, official-source links, and clear launch-status labels instead of copying the
reference branding or claiming its unverified Novro prices.

The public introduction retains the supplied product reference's restrained editorial
hierarchy and generous whitespace. The public documentation is an original long-form API
guide rather than a development manual and remains visually consistent with the site.

## Focused region comparison evidence

- Typography: Geist and Geist Mono use the same weights and line-height hierarchy across
  headings, prices, metadata, badges, and code. Long Chinese copy wraps without clipping.
- Spacing: desktop cards use two stable columns; mobile cards, alerts, and filters stack
  without overlap. There is no horizontal page overflow at either checked viewport.
- Colors: all surfaces use the existing shadcn/ui semantic tokens. Light and dark themes
  keep sufficient text, border, badge, and alert contrast.
- Assets and icons: the supplied references are UI-only and contain no required bitmap
  product imagery. Standard interface symbols use Lucide; official shadcn/ui primitives
  provide inputs, selects, badges, cards, alerts, tabs, and accordions.
- Copy: official model entries, cached/input/output pricing, verification date, source
  links, and the upcoming DeepSeek peak-pricing notice are explicit. GLM planning aliases
  have no invented price and are clearly marked as Novro aliases.

## Interaction and browser evidence

- Search for `DeepSeek` returns `2 / 9` models.
- Vendor, context, and sorting selects are present with accessible labels.
- The default model order follows release date from newest to oldest; entries without
  an independent release date remain at the end.
- The first three models are DeepSeek-V4 Flash (2026-07-31), Kimi K3
  (2026-07-16), and GLM-5.2 (2026-06-16).
- Model cards use locally hosted official DeepSeek, Kimi, and Zhipu brand marks.
- The docs Python tab switches to the Python example.
- The retry accordion expands and shows the bounded retry policy.
- Theme switching was checked in light and dark modes.
- `/docs` contains no Go, pnpm, MySQL, database variable, or `init-db` development text.
- Browser console warning and error checks were clean.
- `/login` exposes OIDC and registration only when returned by `/api/auth/options`.
- A member password login routes to `/console`; first-run setup routes the created
  administrator to `/admin/users`; registration routes the new member to `/console`.
- `/setup`, `/register`, `/login`, and `/console` were checked at 1280 x 720 and
  390 x 844 as applicable. Labels and button names remained accessible, themes switched
  correctly, and no authentication page had horizontal overflow.

## Comparison history

1. The first model pass placed the two unverified Novro aliases between official models.
2. Default ordering was changed so the seven official vendor models appear first and the
   two planning aliases remain visible at the end with prices marked as pending.
3. The official DeepSeek source was rechecked and its upcoming peak-pricing rule was not
   visible on the page.
4. A compact alert was added above the filters. Revised desktop and mobile captures show
   that it does not alter the two-column card rhythm, overlap content, or create overflow.
5. Homepage review found that the 72px launch headline dominated the first screen and the
   copy read like a development status report. The headline was reduced to 52px desktop
   and 32px mobile, then rewritten as a concise product statement. Version, roadmap, and
   implementation-status labels were replaced with customer-facing product language.
6. The first mobile revision left the last character of the headline on its own line.
   Reducing the mobile title to 32px keeps the full heading on one balanced line at
   390 x 844, with no horizontal overflow.

## Findings

No actionable P0, P1, or P2 differences remain.

The page intentionally does not reproduce vendor brand logos because no approved Novro
brand assets or reusable official logo files were supplied. Neutral standard icons keep
the catalog identifiable without presenting approximated logos as official assets.

## Follow-up polish

- P3: Add approved vendor logo assets after their usage rights and files are defined.
- P3: Add active-section highlighting to the docs table of contents as the guide grows.

final result: passed
