# Design QA

## Comparison Target

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-898a8c0e-e888-484c-baaf-2c41b63a1221.png`
- Browser-rendered implementation: `C:\Users\qxnm\Data\Code\novro\tmp\browser-email-desktop.png`
- Side-by-side comparison: `C:\Users\qxnm\Data\Code\novro\tmp\browser-email-comparison.png`
- Route: `/admin/email`
- Viewport: 2048 x 967 desktop
- State: authenticated administrator, light theme, enabled and configured SMTP channel, saved password withheld from the browser

## Full-View Comparison Evidence

The source and implementation were opened together and reviewed again in the combined comparison image. The source is used as an information-architecture reference rather than a visual clone, per the product requirement. Novro preserves the useful host, port, transport security, username, sender address, password-retention, and save concepts while reorganizing them into the existing console shell.

The implementation intentionally differs through Novro's fixed desktop sidebar, compact status header, two-column card layout, immediate enabled/configured status, and a separate test-email workflow. These changes are consistent with the surrounding console rather than resembling the reference product.

The source's certificate-verification bypass and forced AUTH LOGIN controls are intentionally not reproduced. Novro verifies SMTP certificates, supports normal SMTP authentication negotiation, and rejects unencrypted SMTP when enabled in production.

## Focused Region Evidence

The SMTP connection and sender-authentication regions are readable in the combined image. Labels, supporting copy, input grouping, icon alignment, control height, borders, and card spacing are internally consistent. A separate crop was not needed because the important form controls remain legible at the comparison image's original resolution.

The lower enabled-state and test-email cards are not shown in the saved desktop viewport. Their primary interaction states were exercised directly in the browser and are recorded below; they are not being claimed as additional visual-comparison evidence.

## Findings

- No remaining P0, P1, or P2 visual or interaction findings were identified for the desktop source-comparison state.
- Residual test gap: the browser accepted the 390 x 844 viewport override, but its local-page security policy then blocked supplemental screenshot, layout-metric, dark-theme, and console-log capture. No alternate browser surface was used. These supplemental states are not claimed as visually verified in this report.

## Required Fidelity Surfaces

- Fonts and typography: Novro's existing Geist typography, weights, line heights, zero letter spacing, and compact admin-page hierarchy remain consistent with the rest of the console. Labels and supporting copy wrap cleanly in the captured desktop state.
- Spacing and layout rhythm: the implementation uses the console's constrained content width, 8px-or-less surface radii, aligned form rows, predictable five-unit gaps, and a deliberate primary/secondary column split. The denser grouping is an intentional Novro adaptation of the reference's sparse form.
- Colors and visual tokens: neutral foreground, background, border, muted, success, and destructive tokens use the existing light/dark theme system. The light capture has clear label, helper-text, border, and status contrast.
- Image quality and asset fidelity: neither the source nor implementation requires raster product imagery. Visible controls use the repository's Lucide icon family; no CSS art, custom SVG substitutes, or placeholder imagery were introduced.
- Copy and content: Chinese labels are concise and specific to registration verification. Security-sensitive behavior is stated accurately: saved passwords are not returned, blank password input preserves the stored credential, and test mail uses the most recently saved configuration.

## Primary Interactions Checked

- Encryption selector opened by keyboard and changed from STARTTLS to SSL/TLS.
- Saving returned: `SMTP 配置已保存，新的注册验证码邮件会立即使用这组设置。`
- Test delivery returned: `测试邮件已发送到 admin@example.com，请检查收件箱。`
- Turning the enabled switch off immediately disabled the test button and showed its guidance; turning it back on restored the button without saving the disabled state.
- Password visibility changed from `password` to `text` and back to `password`; the synthetic QA-only value was cleared and never saved.
- The browser viewport override was reset before handoff.
- Console-log capture was attempted after the interaction pass but was blocked by the browser's local-page security policy; automated lint, typecheck, tests, production build, Go tests, and Go vet all passed.

## Comparison History

- Pass 1: the desktop source and implementation were compared. No actionable desktop visual mismatch was found because the different layout is an explicit product decision, not fidelity drift.
- Pass 1 interaction finding [P2]: disabling the unsaved form state did not immediately disable the test-email button because it read the last persisted enabled value.
- Fix: the test button and its guidance now follow `form.enabled`, while persistence still changes only after Save.
- Pass 2 browser evidence: with `aria-checked=false`, the test button had the native `disabled` attribute and the disabled-state guidance was visible; restoring `aria-checked=true` removed the disabled state.
- Pass 2 comparison: no remaining actionable P0/P1/P2 finding in the captured desktop source-comparison state.

## Follow-up Polish

- Capture 390 x 844 light and dark states when the in-app browser permits local-page screenshots, then append them as supplemental responsive evidence.

final result: passed

## All Public Page Heading Scale QA (2026-08-08)

### Comparison Target

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-38c2f4a5-6c3c-4ebd-83be-fbf9bacbfa99.png`
- Implemented routes: `/`, `/models`, `/docs`
- Viewports: default desktop viewport and 390 x 844 mobile
- State: public, signed-out, light theme

### Findings

- No remaining P0, P1, or P2 visual, responsive, accessibility, or interaction findings were identified.
- All public display headings that were above the requested reference size now use the same responsive scale: `text-xl sm:text-2xl xl:text-3xl`.
- Compact console headings, card titles, and data values were intentionally left unchanged because they are not display headings and are already below the requested size.

### Required Fidelity Surfaces

- Fonts and typography: public route `h1`/`h2` headings use the existing Geist stack, semibold weight, and matching 20px mobile / 30px wide-desktop type sizes.
- Spacing and layout rhythm: route structures and surrounding content remain unchanged; the homepage mobile viewport has no horizontal overflow.
- Colors and visual tokens: no color, border, radius, or theme token changed.
- Image quality and asset fidelity: no image asset was involved in this typography-only change.
- Copy and content: all route copy remains unchanged.

### Comparison History

- Pass 1: scanned all frontend source for heading sizes above the requested 30px level.
- Pass 2: changed the two remaining public page `h1` elements and reran lint, typecheck, tests, and production build successfully.

final result: passed

## Homepage Heading Scale Consistency QA (2026-08-08)

### Comparison Target

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-38c2f4a5-6c3c-4ebd-83be-fbf9bacbfa99.png`
- Browser-rendered mobile implementation: `C:\Users\qxnm\Data\Code\novro\tmp\home-headings-mobile-latest.png`
- Route: `/` via local Next.js preview at `http://127.0.0.1:3001/`
- Viewports: default desktop viewport and 390 x 844 mobile
- State: public, signed-out, light theme

### Findings

- No remaining P0, P1, or P2 visual, responsive, accessibility, or interaction findings were identified.
- The four remaining homepage section headings now use the same responsive size scale as the requested main headline: 30px at the wide desktop breakpoint, 24px at the small desktop/tablet breakpoint, and 20px on mobile.
- Homepage data values and compact card titles intentionally remain smaller or denser because they are information labels, not section display headings.

### Required Fidelity Surfaces

- Fonts and typography: all homepage `h1`/`h2` elements use the existing Geist stack, semibold weight, and matching responsive size classes; measured desktop headings are 30px and mobile headings are 20px.
- Spacing and layout rhythm: the page keeps the existing section spacing and card layout; the 390px check reported `scrollWidth === 375` and no horizontal overflow.
- Colors and visual tokens: no color, border, radius, or theme token changed.
- Image quality and asset fidelity: no image asset was involved in this typography-only change.
- Copy and content: all homepage copy remains unchanged.

### Comparison History

- Pass 1: measured all homepage `h1`/`h2` elements at desktop and mobile after the unified scale change; all matched the requested size family and no actionable P0/P1/P2 issue remained.

final result: passed

## Homepage Headline Scale QA (2026-08-08)

### Comparison Target

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-38c2f4a5-6c3c-4ebd-83be-fbf9bacbfa99.png`
- Browser-rendered desktop implementation: `C:\Users\qxnm\Data\Code\novro\tmp\home-headline-desktop-2048.png`
- Browser-rendered mobile implementation: `C:\Users\qxnm\Data\Code\novro\tmp\home-headline-mobile-390.png`
- Focused combined comparison: `C:\Users\qxnm\Data\Code\novro\tmp\home-headline-comparison.png`
- Route: `/` via local Next.js preview at `http://127.0.0.1:3001/`
- Viewports: 2048 x 967 desktop and 390 x 844 mobile
- State: public, signed-out, light theme

### Full-View Comparison Evidence

The source screenshot's red-boxed homepage headline was compared with the updated desktop capture at the same 2048px viewport width. The implementation keeps the badge, copy, CTA row, overview panel, and lower model/status sections in their existing positions while reducing only the headline's responsive type scale.

### Focused Region Evidence

The combined comparison image places the source headline crop beside the updated headline crop. The source renders at approximately 60px (`xl:text-6xl`); the updated desktop headline renders at 30px (`xl:text-3xl`) with a 37.5px line box. The mobile capture renders the same headline at 20px with a single clean line at the tested 390px viewport.

### Findings

- No remaining P0, P1, or P2 visual, responsive, accessibility, or interaction findings were identified for the requested headline change.
- The smaller desktop headline is an intentional deviation from the supplied screenshot and directly satisfies the user's request; no surrounding typography was changed.

### Required Fidelity Surfaces

- Fonts and typography: the existing Geist stack, semibold weight, leading, and zero letter spacing remain unchanged; only responsive sizes changed to `text-xl`, `sm:text-2xl`, and `xl:text-3xl`.
- Spacing and layout rhythm: desktop title bounds were 896px wide by 37.5px high; the overview panel begins below the hero without overlap. At 390px, `documentElement.scrollWidth === documentElement.clientWidth === 375`, indicating no horizontal overflow after the browser scrollbar is accounted for.
- Colors and visual tokens: the existing neutral light-theme tokens, borders, status green, and button treatments are unchanged.
- Image quality and asset fidelity: the page contains no source-dependent raster imagery; existing Lucide icons and logo treatment remain unchanged.
- Copy and content: the Chinese headline, supporting description, CTA labels, and overview copy remain unchanged.

### Primary Interactions Checked

- Desktop hero links remain visible and enabled below/alongside the resized title.
- Mobile hero links remain visible, keyboard-accessible links and do not overlap the description or overview panel.
- Browser console inspection contained no warning or error entries for the homepage capture.

### Comparison History

- Pass 1: source and updated desktop/mobile captures were compared in the combined focused artifact. No actionable P0/P1/P2 mismatch remained after the headline-only change.

final result: passed

---

# Public Homepage And Return-Home QA (2026-08-07)

## Comparison Target

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-b868ca39-f72a-4516-955c-8f2e06f0595c.png`
- Browser-rendered desktop implementation: `C:\Users\qxnm\Data\Code\novro\tmp\home-desktop.png` (latest live verification also captured at `http://localhost:3000/`)
- Browser-rendered mobile implementation: `C:\Users\qxnm\Data\Code\novro\tmp\home-mobile.png`
- Route: `/` (served by the local Next.js preview at `http://localhost:3001/`; production default remains `http://localhost:3000/`)
- Viewports: default desktop browser viewport and 390 x 844 mobile override
- State: public, signed-out, light theme

## Full-View Comparison Evidence

The reference image was used as an information-architecture reference for a customer-facing overview rather than a brand clone. The public homepage now presents the same useful hierarchy through Novro's real scope: overview status, model and pricing summary, integration status, and an announcement panel. The existing public model catalog and API documentation remain one click away.

The console shell now has three explicit return-home affordances: the desktop sidebar brand, the desktop/mobile top-bar home control, and the public header's 首页 navigation item. Each uses the root route `/`, which resolves to `http://localhost:3000/` in the requested deployment.

## Focused Region Evidence

The desktop screenshot confirms the above-the-fold hero, platform overview strip, status badge, and two primary CTAs. The mobile screenshot confirms the same hierarchy stacks without clipping and that navigation collapses into the existing accessible sheet. A focused crop was not needed because the public homepage's key regions are legible in the captured viewport and its remaining sections are straightforward continuation of the same layout.

## Findings

- No remaining P0, P1, or P2 visual, interaction, accessibility, or responsive findings were identified.
- An unrelated process initially occupied port 3000, so the first development capture used port 3001. The latest production preview was restarted on port 3000 and the requested URL was rechecked with the revised homepage and zero browser warning/error entries.

## Required Fidelity Surfaces

- Fonts and typography: existing Geist typography, compact labels, strong page title, readable supporting copy, and zero letter spacing are preserved. Chinese headings wrap cleanly at 390px.
- Spacing and layout rhythm: the hero, overview strip, model card grid, status/announcement cards, and existing lower sections use the repository's constrained width, consistent section gaps, and 8px-or-less card radii. Mobile reported `documentWidth === 375` and `scrollWidth === 375` with no horizontal overflow.
- Colors and visual tokens: the implementation uses the existing shadcn background, foreground, muted, border, secondary, success, and dark-theme tokens. The public page remains neutral and operational rather than a single-hue marketing surface.
- Image quality and asset fidelity: the reference and implementation do not require raster product imagery. Visible controls use the existing Lucide icon family; no custom SVG, CSS art, emoji, or placeholder asset was introduced.
- Copy and content: all new homepage copy describes actual Novro capabilities and clearly separates public information from signed-in balance, usage, and API-key data.

## Primary Interactions Checked

- Public header 首页, 模型, and 接入文档 links resolved to `/`, `/models`, and `/docs`.
- Hero buttons resolved to `/login` and `/docs`.
- The desktop and mobile home controls are keyboard-accessible links with accessible labels/titles and target `/`.
- Mobile navigation collapsed into the existing sheet at 390px and the page had no horizontal overflow.
- Browser console capture contained no warning or error entries for the public homepage.

## Comparison History

- Pass 1: the public root already exposed models, pricing, and docs, but its first viewport did not express the reference's customer overview hierarchy and the console had no explicit return-home action.
- Fix: added a customer-oriented platform overview strip, model/pricing summary, integration status and announcement panels; added 首页 and 返回主页 affordances pointing to `/`.
- Pass 2: desktop and 390px mobile captures showed the revised hierarchy with no actionable P0/P1/P2 mismatch. The mobile viewport was reset before handoff.

## Follow-up Polish

- The existing lower homepage sections intentionally remain available for deeper model, integration, and team-management context below the first viewport.

final result: passed

## Final Implementation Check (2026-08-07)

- Final real preview: `http://localhost:3000`, Next.js rewrites to the Go API at `http://127.0.0.1:8080`.
- Database evidence: `go run ./cmd/novro migrate` applied the referral setting migration successfully twice; `go run ./cmd/novro check-db` returned `database connection is ready`.
- Final browser evidence: `C:\Users\qxnm\Data\Code\novro\tmp\referral-profile-desktop.png` at 2048 x 1884 and `C:\Users\qxnm\Data\Code\novro\tmp\referral-profile-mobile-final.png` at 390 x 844. The mobile DOM reported `documentWidth === 390` and no horizontal overflow; profile and referral cards were 358px wide inside the 390px viewport.
- Interaction evidence: details drawer, reward/invitation tabs, administrator save at 12.35%, empty-value validation, and logout redirect to `/login` with a subsequent `401` session check were exercised against a disposable stateful preview. The disposable preview was stopped before the final real preview was restored.
- Final result: passed

---

# Referral Cashback Design QA (2026-08-07)

## Comparison Target

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-273ea5ab-c3ce-48f1-9ab3-bacfe4aa5f01.png`
- Browser-rendered desktop implementation: `C:\Users\qxnm\Data\Code\novro\tmp\referral-profile-desktop-card.png`
- Browser-rendered desktop drawers: `C:\Users\qxnm\Data\Code\novro\tmp\referral-details-drawer-desktop.png` and `C:\Users\qxnm\Data\Code\novro\tmp\referral-invitations-drawer-desktop.png`
- Browser-rendered mobile implementation: `C:\Users\qxnm\Data\Code\novro\tmp\referral-profile-mobile-top.png`
- Browser-rendered mobile drawers: `C:\Users\qxnm\Data\Code\novro\tmp\referral-details-drawer-mobile.png` and `C:\Users\qxnm\Data\Code\novro\tmp\referral-invitations-drawer-mobile.png`
- Browser-rendered dark-theme states: `C:\Users\qxnm\Data\Code\novro\tmp\referral-profile-mobile-dark.png` and `C:\Users\qxnm\Data\Code\novro\tmp\referral-details-drawer-mobile-dark.png`
- Route: `/console/profile`
- Viewports: 1280 x 720 desktop and 390 x 844 mobile
- State: authenticated member, populated referral summary, populated reward and invitation histories, light and dark themes

## Full-View And Focused Comparison Evidence

The source and current browser-rendered profile were opened together in `C:\Users\qxnm\Data\Code\novro\tmp\referral-comparison-full.png`. The full view confirms that the recommendation plan now follows the basic-information form and remains inside Novro's existing console shell instead of displacing the primary profile workflow.

The source panel and the rendered Novro card were also normalized into one focused comparison artifact at `C:\Users\qxnm\Data\Code\novro\tmp\referral-comparison-card.png`. Both use the same left-to-right hierarchy: recommendation identity, three financial/invitation metrics, invitation URL, and copy action. Novro intentionally wraps the description at its narrower console content width and adds a second icon-only action for the requested details drawer. Those are responsive and functional adaptations, not unresolved fidelity drift.

## Findings

- No remaining P0, P1, or P2 visual, interaction, accessibility, or responsive findings were identified.
- Residual QA limitation: the copy action changed to its visible success state and `aria-label="邀请链接已复制"`, while the in-app Browser's virtual clipboard returned an empty value and could not paste it back. The read-only invitation field itself still contained `http://localhost:3000/register?ref=ABCD1234EF56`; clipboard contents are therefore not claimed as independently verified.

## Required Fidelity Surfaces

- Fonts and typography: the rendered card uses Novro's existing Geist hierarchy, tabular figures, zero letter spacing, readable muted copy, and consistent desktop/mobile line heights. The narrower console card wraps the explanatory sentence cleanly without truncating the rate or reward meaning.
- Spacing and layout rhythm: the recommendation panel retains the source's horizontal rhythm on desktop, stacks its metrics and controls predictably below `xl`, and follows the basic-information card with the same five-unit section gap. The 390 x 844 page and drawer both reported `scrollWidth === 390`, with no clipped or overlapping controls.
- Colors and visual tokens: border, background, foreground, muted, focus, success, and overlay colors use the existing shadcn/ui semantic tokens. Light and dark captures preserve hierarchy and readable reward emphasis without introducing a separate referral palette.
- Image quality and asset fidelity: the source contains no raster product imagery. Visible share, copy, drawer, gift, user, and close controls use the repository's Lucide icon family; no custom SVG, CSS art, emoji, or placeholder asset was introduced.
- Copy and content: title, 10% rate, pending reward, total reward, invitation count, invitation URL, reward rows, invitation rows, timestamps, and empty-state guidance are coherent and specific to the referral workflow. Invitee email addresses, wallet IDs, and payment order numbers are not exposed.

## Primary Interactions Checked

- The recommendation card is positioned directly below basic information on desktop and mobile.
- The details icon opens a right-side drawer on desktop and a full-width drawer at 390 x 844.
- `返现记录` and `邀请记录` both switch correctly and show the expected populated rows.
- Reward rows show member, username, recharge amount, reward amount, and credited time; invitation rows show display name, username, and joined time.
- Light and dark profile/card/drawer states were captured. The preview was restored to light theme before handoff.
- The copy button changed to its check-mark success state and accessible copied label. Clipboard-content inspection remained unavailable as described above.
- Browser console inspection contained only the React development-tools notice and HMR connection message; no page warning or error was present.

## Comparison History

- Pass 1: rejected because stale Next.js chunks left the browser on the loading shell.
- Pass 2: the disposable API and Next.js preview were restarted, but the earlier browser session could not produce claimable same-state evidence; the result stayed blocked.
- Pass 3: the in-app Browser connected to the running profile preview. Desktop card, reward drawer, invitation drawer, and copied-button captures showed no actionable P0/P1/P2 mismatch.
- Pass 4: 390 x 844 light reward/invitation drawers, mobile dark profile/drawer, exact viewport width, zero horizontal overflow, and clean browser console evidence were captured. The focused and full combined comparison artifacts were opened and reviewed; no earlier blocking finding remained.

final result: passed
