# Hidden Billing Group Authorization Design QA

## Comparison Target

- Source visual truth: `C:\Users\qxnm\AppData\Local\Temp\codex-clipboard-1e4c54d1-c12f-42b4-9fbf-38d618356d09.png`
- Browser-rendered desktop implementation: `C:\Users\qxnm\Data\Code\novro\tmp\billing-groups-desktop-final.png`
- Source-size desktop implementation: `C:\Users\qxnm\Data\Code\novro\tmp\billing-groups-desktop-2048-final.png`
- Browser-rendered mobile implementation: `C:\Users\qxnm\Data\Code\novro\tmp\billing-groups-mobile-final.png`
- Full-view comparison: `C:\Users\qxnm\Data\Code\novro\tmp\billing-groups-reference-comparison.png`
- Focused drawer comparison: `C:\Users\qxnm\Data\Code\novro\tmp\billing-groups-drawer-comparison.png`
- Route: `/admin/billing-groups`
- Viewports: 1280 x 720 desktop, 2048 x 1132 source-size desktop, and 390 x 844 mobile
- State: authenticated administrator, light theme, hidden proxy group edit drawer open, two ordinary members authorized in the reversible QA mock

## Full-View Comparison Evidence

The source and browser-rendered implementation were opened together in the full-view comparison. Both use the existing Novro administration shell, a right-side `编辑计费分组` drawer, the same group fields, a hidden-group control, an authorization region, and a persistent bottom save action.

The source only annotates the intended authorization region with placeholder user names. The implementation realizes that region as a searchable, keyboard-accessible checkbox list with a selected-user count. This is an intentional functional completion of the source rather than visual drift.

## Focused Region Evidence

The focused comparison normalizes the right-side drawer to 448 px. The source and implementation have matching drawer proportions and preserve the same top-to-bottom hierarchy. The implementation's authorization rows remain legible at this width and show both display name and account identity.

No additional crop was needed for the mobile state because the complete 390 x 844 drawer, user rows, and save footer are all legible in the saved mobile screenshot.

## Findings

- No actionable P0, P1, or P2 visual, responsive, accessibility, or interaction findings remain.
- The mock source does not specify search, selected count, disabled-user treatment, or empty/loading states. The implementation uses existing Novro shadcn/ui patterns for these production-required states.
- The source shows a sample multiplier of 0.1 while the QA mock uses 0.5. This is test data, not a visual or behavioral mismatch.

## Required Fidelity Surfaces

- Fonts and typography: existing Geist typography, weights, line heights, zero letter spacing, label hierarchy, muted helper text, truncation, and Chinese wrapping are preserved. The compact drawer uses form-scale type rather than display headings.
- Spacing and layout rhythm: the final desktop drawer is 448 px wide, matches the source proportion, and uses fixed header, independently scrolling form content, and fixed footer rows. At 390 x 844 the drawer fills the viewport, reports no horizontal overflow, and leaves a clear gap between the last user row and the save footer.
- Colors and visual tokens: background, foreground, muted text, borders, overlay, focus ring, checkbox, badge, and primary button use the existing shadcn/ui semantic tokens and remain compatible with the existing light/dark theme system.
- Image quality and asset fidelity: the source and implementation contain no product imagery or custom raster assets. Search, edit, close, user, and checkbox controls use the repository's existing icon and primitive libraries; no placeholder graphics or handcrafted SVG substitutes were added.
- Copy and content: `隐藏分组`, `授权用户`, independent-authorization guidance, selected count, search placeholder, and user identities clearly express that each hidden group has its own authorized-user list and that administrators do not require assignment.

## Primary Interactions Checked

- Opened the proxy-group edit drawer and changed its reversible in-memory QA selection from only `proxy.user` to `proxy.user` plus `business.user`.
- Saved the change, confirmed the table's proxy-group authorization count changed from 1 to 2, reopened the drawer, and confirmed both checkboxes remained selected in the QA mock.
- Confirmed the B-side group and proxy group are represented as separate hidden groups with independent authorization lists.
- Confirmed the save action remains fully visible at 1280 x 720 and 390 x 844.
- Confirmed the 390 x 844 drawer is full width and has no horizontal overflow or footer/user-row overlap.
- Opened `创建用户` and the ordinary-member edit drawer on `/admin/users`; neither contains `允许访问隐藏分组`, related hidden-access text, or the former permission checkbox.
- Browser console and exception events were collected after a fresh reload; no warnings or errors were present.

## Comparison History

- Pass 1 finding [P2]: the initial desktop edit drawer used a normal flex column, so `保存修改` was pushed below the 720 px viewport.
- Pass 1 fix: changed the drawer to fixed header, `minmax(0, 1fr)` scrolling form content, and fixed footer rows. Post-fix evidence showed the save button at top 672 px and bottom 704 px in the 720 px viewport.
- Pass 2 finding [P2]: the shared Sheet primitive's mobile `w-3/4` left about 98 px of the underlying page visible at 390 px.
- Pass 2 fix: added a billing-group-specific full-width mobile override. Post-fix evidence shows dialog left 0 px, width about 390 px, no horizontal overflow, and a fully visible save button.
- Pass 3 finding [P2]: the initial desktop override remained 384 px wide, narrower than the approximately 448 px reference drawer, reducing authorization-list readability.
- Pass 3 fix: set a scoped 448 px desktop width with explicit Tailwind priority while retaining the full-width mobile rule. Final evidence shows 448 px at 1280 x 720 and 2048 x 1132, and about 390 px at 390 x 844.
- Final comparison: the full-view and focused source/implementation artifacts were opened and reviewed after all fixes. No actionable P0/P1/P2 difference remains.

## Follow-up Polish

- None required for handoff. Additional user rows will scroll inside the bounded authorization list without moving the persistent save footer.

final result: passed
