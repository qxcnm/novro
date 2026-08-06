# Design QA

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-a857d24c-f4ad-48ee-b6a9-a622096aca8e.png`,
  `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-3143c202-ecb3-4a56-a9af-cfa793eeba97.png`,
  the supplied QuarkMux product-page reference, and the current account/navigation references
  `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-a2240999-5676-4527-9827-dec0ef4e67d4.png`,
  `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-88a9b95f-6b6d-4b34-ae7a-a40ae31d1a68.png`, and
  `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-448c6bcc-1654-4077-8025-6b339acbe3f9.png`.
- Implementation: `http://127.0.0.1:3000/`, `/models`, and `/docs`.
- Implementation screenshots: `tmp/qa/models-desktop-light-final.png`,
  `tmp/qa/models-mobile-light-final.png`, `tmp/qa/models-mobile-dark-final.png`,
  `tmp/qa/home-desktop-light-final.png`, and `tmp/qa/docs-mobile-light-final.png`.
  The current authenticated account capture is `tmp/qa/account-dashboard-desktop.png`, with
  focused content evidence in `tmp/qa/account-dashboard-content.png`. The latest persistent-shell
  captures are `tmp/qa/console-desktop-current.jpg`, `tmp/qa/console-mobile-current.jpg`, and
  `tmp/qa/api-key-mobile-current.jpg`.
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
- The authenticated console sidebar and administrator user management were rechecked at
  1280 x 800 and 390 x 844 after the navigation refresh. Desktop uses a fixed 240px
  sidebar; mobile replaces it with a left navigation drawer and has no page-level
  horizontal overflow.
- The current production build was rechecked against the current Go backend and an isolated MySQL
  8.4.9 database at 1280 x 800 and 390 x 844. Navigating through account, API documentation,
  user management, Key audit, providers, and model routes kept the desktop sidebar visible,
  never rendered the full-screen `正在加载控制台...` state, and produced no horizontal overflow.
- On mobile, the persistent desktop sidebar correctly becomes a navigation drawer. Selecting
  `/console/docs` closes the drawer, retains the console header, and only replaces the content
  region. The account screen, drawer, and API Key modal all fit the 390px viewport.
- Create and reset forms use dialogs, status changes use a confirmation dialog, and user
  details/editing use a right drawer. Dialogs remained inside the mobile viewport, the
  details drawer stayed independently scrollable, and no write action was submitted during QA.
- The authenticated account page displayed a balance of `¥43.71`, safe API Key prefixes, and
  a one-time full key after creation. Copying placed the exact full key in the clipboard and the
  active count changed from one to two.
- The current account page created a new Key against the current backend. The full 47-character
  key appeared only in the save dialog, the list retained only its safe prefix, and the copy
  button placed the exact full value in the browser clipboard. No browser console warnings or
  errors were recorded during the current desktop/mobile navigation and Key flow.
- On 2026-08-06, the current Go service, Next.js console, and migrated cloud MySQL were verified
  together rather than against a mocked or stale backend. A temporary member registered and
  received an empty wallet, created and copied a one-time 47-character Key, and used it to obtain
  a `200` model list. After a temporary provider and route were added, the public model appeared
  in `/v1/models`; an upstream `502` reserved 360 micros and refunded the same 360 micros, leaving
  the 10,000-micro balance unchanged. Revoking the Key from the audit page made the same Key return
  `401` immediately. The temporary user, Key, provider, route, wallet entries, and session were
  removed after verification.
- The live console was also rechecked with an explicit 390 x 844 viewport. The page width remained
  390px with no out-of-bounds buttons; the navigation drawer was about 293px wide, and the 801px
  Key-audit table stayed inside a 358px local horizontal scroller instead of widening the page.
- Live QA exposed a typed-nil OIDC assembly bug: an unconfigured `*OIDCClient` assigned to an
  interface caused the login options endpoint to report enterprise sign-in as enabled. The service
  assembly now preserves a true nil interface, a regression test covers both states, and the live
  endpoint returns `oidc_enabled=false` when OIDC is not configured.
- Navigating from `/console` to `/admin/users` kept the sidebar mounted. The API documentation
  menu targets `/console/docs`; the current build completed this route and all other console
  menu transitions without showing the full-screen authentication loader.

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

No actionable P0, P1, or P2 differences were visible in the current desktop or mobile console
captures. The sidebar/header persistence behavior, local content replacement, mobile drawer,
one-time Key presentation, and copy interaction are directly verified against the current build.

The page intentionally does not reproduce vendor brand logos because no approved Novro
brand assets or reusable official logo files were supplied. Neutral standard icons keep
the catalog identifiable without presenting approximated logos as official assets.

## Follow-up polish

- P3: Add approved vendor logo assets after their usage rights and files are defined.
- P3: Add active-section highlighting to the docs table of contents as the guide grows.

final result: passed
