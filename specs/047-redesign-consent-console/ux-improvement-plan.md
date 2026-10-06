# AIB Consent Console: UX improvement plan

Oct 4, 2026 · @Jan Brennenstuhl

## Verdict

The redesign on `047-redesign-consent-console` is a sound engineering base with a generic, table-first interface on top, and your doubts are justified. The stack (React 19, Tailwind 4, owned Radix primitives, Storybook 10 with accessibility gating) should stay. The visual language, page composition and several interaction patterns should be redone.

This review covers commit `2e6755b`. It is based on the source, the tokens and your observations. The app could not be run here because the package registry refused a dependency download, so no finding below rests on a rendered screenshot.

Most of what you saw traces back to nine concrete causes in the code:

| What you saw | Cause in the code |
| --- | --- |
| "Show more / less" everywhere, doing nothing | `TruncatedText` always renders its toggle and never checks whether the text overflows. It is used in 11 files. |
| Rows jump in Approvals | `InlineApprovalActions` mounts three bordered radio cards, a scope preview and the scope editor inside every table cell, before any decision is chosen. |
| Dark mode too contrasty | All neutrals have zero chroma. `--border` is lightness 0.58 on a 0.16 background and is used for every card, button, badge and alert. Status badges are solid fills at lightness 0.78. |
| Warning barely visible | Every `Alert` variant shares `border-border bg-card`. Only the 20 px icon carries colour. |
| Badges float and look too round | `Badge` is `rounded-full` and sits in its own wrapping row under the text it describes. |
| Logo looks padded | The wordmark SVG has a 500 × 60 viewBox. The ink covers x 47–454 and y 15–54, so 9.5 % of each side and 25 % of the top are empty. |
| Clicking highlights the whole Connections box | The page wrapper has `tabIndex={-1}` with `focus:ring-2` instead of `focus-visible`. |
| Permission unfold out of sight | Console `<main>` has no maximum width. The accordion chevron is pushed to the far right edge by `justify-between`. |
| "View" and "Revoke" do not look like buttons | Both use the `ghost` variant, which has no border and no fill. Rows themselves are not clickable. |

Five changes carry most of the improvement:

1. **Consent becomes one compact card** with three zones (who, what, decide) and the Allow button always visible. Target: no scrolling at 1280 × 720 with up to three permission groups.
2. **Tables go.** Agents and Connections become card grids with a list toggle. Approvals becomes a list with a detail panel, so nothing moves when you decide.
3. **One layout system.** A centred 1120 px container on a 12-column grid, one page header with an icon toolbar, one entity anatomy reused everywhere.
4. **A re-tuned palette.** Tinted neutrals, three border levels, soft status colours, a stronger accent, and a dark theme that separates surfaces by lightness instead of bright outlines.
5. **A quieter shell.** Three navigation items, an icon to collapse the sidebar, Settings in the user menu.

One process point comes first. `DESIGN_PRINCIPLES.md` requires an accepted ADR before any change to the visual direction, and ADR 038 fixes the current palette, CSS-only motion and the 640 px decision column. ADR 038 is still a draft on this branch, so work package 0 amends ADR 038 and the feature 047 spec in place. No new ADR is needed.

## Design principles

Every view is judged against these eight rules. They replace the "A focused security tool" section of `DESIGN_PRINCIPLES.md`.

1. **The decision is always on screen.** On consent and approval views the primary action is visible without scrolling at 1280 × 720 and stays pinned on smaller screens.
2. **Show what changes the decision; link the rest.** Who is asking, what they get, for how long, and any risk signal are always visible. Documentation links, client IDs and raw scopes sit one click away in a popover or dialog.
3. **Nothing moves under the pointer.** No control changes the height of a row or card in place. Detail opens in a panel, a dialog or a region whose space is already reserved.
4. **No dead controls.** A control renders only when it does something. A "show more" toggle appears only when text is measured as clipped.
5. **Controls look like what they are.** Actions are buttons with a visible boundary. Navigation is a link or a whole clickable card. Status is a badge and is never clickable.
6. **Colour carries meaning.** The accent marks the one primary action and the current selection. Semantic colours mark status and risk, always with an icon or word. Everything else is neutral.
7. **One anatomy for every entity.** Agents, connections, approvals and standing decisions share one structure: icon, name, one supporting line, one status, at most two visible actions.
8. **Motion explains a change.** 120–200 ms for feedback, up to 320 ms for a celebratory or illustrative moment, none under reduced motion.

The risk signal in rule 2 deserves emphasis. An eye-tracking study of authorization dialogs found that users habitually skip permission lists, and that forcing attention to everything did not make them refuse dangerous permissions more often ([PoPETs 2017](https://petsymposium.org/popets/2017/popets-2017-0014.php)). The practical consequence is to keep the screen calm and make the one or two risky facts impossible to miss.

## Foundations

The token layer needs a second pass before any view is rebuilt, because the dark-mode harshness, the floating badges and the invisible warnings all originate in `theme.css` and four primitives.

### Colour

The current palette is pure grey plus one blue. Three changes make it calmer in dark mode and more distinctive in both.

- **Tint the neutrals.** Give every neutral a small chroma (0.003–0.012) at hue 260 so surfaces relate to the accent.
- **Split borders into three roles.** Today `--border` (3.5:1 in light, 4.5:1 in dark) outlines everything. Only form controls need 3:1 under WCAG 1.4.11. Cards, dividers and filled buttons take a quiet border.
- **Make status colours soft by default.** A tinted background with coloured text replaces solid fills. Solid stays for the primary and destructive-confirm buttons only.

Proposed dark values, with contrast computed against the new background. The light theme follows the same roles; its values are in work package 1.

| Token | Current dark | Proposed dark | Contrast on background |
| --- | --- | --- | --- |
| `--background` | `oklch(0.16 0 0)` | `oklch(0.175 0.006 260)` | n/a |
| `--card` | `oklch(0.21 0 0)` | `oklch(0.21 0.008 260)` | n/a |
| `--popover` (raised) | `oklch(0.21 0 0)` | `oklch(0.245 0.009 260)` | n/a |
| `--foreground` | `oklch(0.96 0 0)` | `oklch(0.93 0.005 260)` | 15.4:1 (was 17.3:1) |
| `--muted-foreground` | `oklch(0.78 0 0)` | `oklch(0.72 0.012 260)` | 7.7:1 (was 9.7:1) |
| `--border-subtle` (new; dividers, card edges) | `oklch(0.32 0 0)` as `--border-soft` | `oklch(0.275 0.01 260)` | 1.3:1 |
| `--border` (buttons, popovers) | `oklch(0.58 0 0)` | `oklch(0.33 0.012 260)` | 1.6:1 (was 4.5:1) |
| `--border-control` (new; inputs, checkbox, radio) | uses `--border` | `oklch(0.54 0.012 260)` | 3.8:1 |
| `--primary` | `oklch(0.78 0.10 250)` | `oklch(0.70 0.15 262)` | 7.0:1 |
| `--primary-soft` (new; selection, active nav) | none | `oklch(0.30 0.06 262)` | text on it 7.3:1 |
| Status soft background | solid `oklch(0.78 0.10 h)` | `oklch(0.28 0.05 h)` with text `oklch(0.82 0.11 h)` | text on it 8.0–8.6:1 |

These are starting values that pass WCAG AA on paper. Extend `contrast.test.ts` to cover the new pairs, then tune by eye in Storybook.

The accent gets more presence without adding a second brand colour:

- Light `--primary` moves from `oklch(0.48 0.14 255)` to `oklch(0.50 0.19 262)` (white text on it 6.2:1).
- `--primary-soft` fills the active navigation item, selected cards, the selected approval row and icon tiles.
- Agents and services without a logo get a deterministic tint from a fixed set of eight hues, derived from their ID, so each has a recognisable colour.

### Layout grid and spacing

- Console content sits in a centred container with `max-width: 1120px` and 24 px side padding (16 px below 768 px).
- Inside it, a 12-column grid with 24 px gutters. List pages span 12. Detail pages use 8 + 4.
- Card grids use `repeat(auto-fill, minmax(320px, 1fr))` with a 16 px gap.
- Spacing uses the existing 4 px base but only these steps: 4, 8, 12, 16, 24, 32, 48. Inside a component 4–12, between components 16–24, between sections 32–48.
- Running text is capped at `70ch`.

### Type

Keep Zalando Sans for headings, Inter for text and JetBrains Mono for identifiers. Set the console base to 14 px and the consent card to 15 px. Use one scale: 12, 13, 14, 16, 20, 24. Page titles are 20 px semibold, not 24. Dates and counts use tabular figures in Inter, not the mono face.

### Shape

| Element | Current | Proposed |
| --- | --- | --- |
| Badge | `rounded-full` | 6 px |
| Button, input, select | 6 px | 8 px |
| Card, alert, panel | 8 px | 12 px |
| Dialog, consent card | 8 px | 16 px |
| Agent and service avatar | circle | rounded square, 8 px (people stay circular) |

### Badges

- One size: 20 px tall, 12 px medium text, 6 px radius, 6 px horizontal padding.
- Default variant is soft (tinted background, coloured text, no border). Neutral is grey soft. Remove the solid status variants.
- Each badge has a leading 6 px dot or a 12 px icon so colour is never the only signal.
- A badge sits on the title's baseline, directly after the name, or in a fixed right-aligned status slot. It never gets its own wrapping row.
- At most one badge per entity. "Optional" and "Already granted" stop being badges (see Consent).
- Fixed vocabulary: Connected, Needs sign-in, Expired, Unavailable, Low risk, Medium risk, High risk, Critical, Always allowed, Always denied, Required.

### Buttons and alerts

- Primary and destructive buttons currently underline on hover, which reads as a link. Replace with a lightness shift of 0.04 and a 0.98 scale on press.
- Add a `destructive-outline` variant for visible but calm destructive actions such as Revoke and Disconnect.
- Row-level actions use `outline` at 32 px height. `ghost` is reserved for icon buttons with a tooltip.
- `Alert` variants get a soft status background, a matching `--border-subtle`-weight tinted border and a 16 px icon. Padding drops from 16 to 12 px.
- `Alert` gets an `inline` variant with no box (icon and one line of text) for field-level hints.

### Icons

Stay on Lucide. Set the default stroke to 1.75 at 16 px and 1.5 at 20 px and above; the stock 2 px stroke is heavy next to Inter at 14 px. Every navigation item, row action, status and empty state gets an icon. Suggested mapping:

| Concept | Icon |
| --- | --- |
| Agents | `Bot` |
| Connections | `Cable` |
| Approvals | `ShieldCheck` |
| Required permission | `Lock` |
| Verified origin / unverified / local | `BadgeCheck` / `ShieldAlert` / `Laptop` |
| Connected / needs sign-in / expired | `CircleCheck` / `KeyRound` / `ClockAlert` |
| Revoke / disconnect | `ShieldOff` / `Unlink` |
| External link | `ArrowUpRight` |

### Motion and animated icons

The animated tiles in Claude's artifact gallery work because they are few, large and tied to hover. Apply the same restraint: animate about eight icons, not all of them.

- **Where:** the three navigation icons on hover, the three empty-state illustrations, the success check after Allow or Approve, and the plug on a new connection.
- **Console routes:** use [lucide-animated](https://lucide-animated.com). It offers 350+ animated Lucide icons built on Motion, and its source copies into the repo, so the icons stay owned components. The Motion dependency is approved for console routes.
- **Decision routes:** consent and the standalone approval review stay free of Motion. Author their few animations (the success check, the risk callout entrance) as owned SVG components animated with CSS (`stroke-dashoffset` draws, small transforms). The existing `bundle` test project should assert that no decision route imports Motion.
- **List and card changes:** on console routes, use Motion's `AnimatePresence` and `layout` so a revoked card or a decided approval leaves and the rest close the gap smoothly.
- **Navigation transitions:** use the browser View Transitions API as progressive enhancement, so a card grows into its detail header. Browsers without it simply cut.
- **Tokens:** keep 120, 160 and 200 ms. Add `--motion-emphasis: 320ms`. Change the easing from `ease-out` to `cubic-bezier(0.2, 0, 0, 1)`. Exits run at 75 % of the entrance duration. Motion transitions read the same durations and easing from one shared constants module.
- **Reduced motion:** keep opacity and colour changes, drop movement and icon animation. Wrap the console in `<MotionConfig reducedMotion="user">`. The current CSS rule sets every transition to `none`, which also removes helpful fades.

### Logo

Crop both SVGs to their ink. The wordmark becomes `viewBox="47.4 15 406.7 38.5"` and the mark `viewBox="10 17.9 46.2 27.7"`. Size the wordmark by height (18 px in the sidebar, 20 px on the consent screen) and align its left edge to the navigation icons' left edge.

## App shell and navigation

The sidebar should hold three destinations and nothing else that competes with them. Today it stacks a wordmark, a full-width "Collapse sidebar" button, a full-width outlined Search button, four navigation items and an icon-only avatar.

| Element | Today (`ConsoleShell.tsx`, `ConsoleLayout.tsx`) | Change |
| --- | --- | --- |
| Collapse control | Labelled ghost button on its own row | 28 px icon button (`PanelLeft`) in the header row, right of the wordmark. Shortcut `Cmd/Ctrl + B`. When collapsed, the mark swaps to the expand icon on hover. |
| Search | Full-width outlined button above the navigation | Borderless row in muted text: search icon, "Search", and a `⌘K` hint on the right. Icon only when collapsed. |
| Navigation | Agents, Connections, Approvals, Settings | Agents, Connections, Approvals. Active item uses `--primary-soft` with accent text and icon. |
| Pending count | Grey pill | Accent-soft pill when above zero, hidden at zero. |
| Stale notice | Extra line of text under the Approvals item | Tooltip on a small warning icon. The sidebar must not change height. |
| User menu | Avatar icon at the bottom, menu holds name and theme | Full-width row: avatar, name, chevron. Menu holds Settings, Appearance (light, dark, system) and Documentation. |
| Width | 256 px, 80 px collapsed | 240 px, 56 px collapsed |
| Main area | Full width, 16–24 px padding | Centred 1120 px container (see Foundations) |

### Page header

`PageHeader` currently requires a `purpose` sentence on every page. Make it optional and drop it from Agents, Connections and Approvals; the same sentence moves into each empty state, where a new user reads it. The header becomes one row: title on the left, a count in muted text beside it ("Agents · 4"), and the icon toolbar on the right.

### Tabs and actions

Tabs switch between views of the same object and change the URL. Actions change data. They must not share a row or a visual style ([Primer navigation patterns](https://primer.style/product/ui-patterns/navigation/)).

- Use the existing `line` variant of `TabsList` (underline) for in-page views. Retire the pill variant, which looks like a row of outline buttons.
- Page actions sit in the page header's right slot.
- External links are text links with an `ArrowUpRight` icon, never buttons.

### Routes and copy

- The navigation says Agents and Connections, the URLs say `/delegations` and `/sessions`. Rename to `/agents` and `/connections` in the same cutover, with no aliases or redirects. Update the provider callback URL that the Connections view builds, the UI contract and the end-to-end page objects with it.
- User-facing copy prints a raw path twice: "Review or revoke access at /delegations". Replace with a link labelled Agents.
- Connections copy mixes three words for one thing ("Session terminated", "Session token refreshed", "Disconnect"). Use "connection" throughout.

## Shared patterns

Five shared components replace the three TanStack tables and the ad hoc states around them. Build them once in the design system and compose every list page from them.

### Cards or rows

Use cards for Agents and Connections and rows with a detail panel for Approvals.

The evidence is mixed, and the choice should be made knowingly. NN/g finds that cards scan more slowly than lists when items are alike, and suit browsing a mixed set ([Cards](https://www.nngroup.com/articles/cards-component/)). Vercel's Geist and Shopify's Polaris both recommend a list of entity rows over a data table when users find an object and act on it ([Entity](https://vercel.com/geist/entity), [Resource list](https://polaris-react.shopify.com/components/lists/resource-list)). Cards still win here for two reasons: users have fewer than ten agents and recognise them by logo and name, and each card can show state that a three-column table hides. A list view remains available through the toggle and becomes the default above twelve items. Approvals is a queue to work through, so it stays a list.

### `EntityCard` and `EntityRow`

One anatomy in two densities, sharing props.

- **Leading:** 40 px rounded-square avatar (logo, or tinted initial).
- **Title line:** name at 14 px semibold, then at most one badge on the same baseline.
- **Supporting line:** one line of muted text, such as "3 permissions · until revoked".
- **Meta slot:** optional row of small service icons or a date.
- **Actions:** at most two visible. In a card they sit in a footer row; in a row they sit on the right.
- **Navigation:** when the entity has a detail page, the whole surface is one link. Action buttons sit above the link layer and stop propagation.
- **States:** hover raises the border to `--border` and shows a subtle background shift. Focus shows the ring on the card. Busy dims the content and shows a spinner in the action.
- **Height:** fixed per density. Long names truncate with an ellipsis and a tooltip; they never wrap or expand in place.

### `CollectionToolbar`

A row of 32 px icon buttons in the page header, in the style of the gallery in your screenshots.

- **Search:** an icon that expands to a 240 px field in place. `/` focuses it and `Esc` clears and collapses it. It filters as you type.
- **Sort:** an icon that opens a menu (Name, Recently changed, Expiring soonest). The active choice shows a check.
- **Filter:** an icon that opens a popover. Render it only when the page has a real facet (connection state, approval risk or agent). Applied filters show as removable chips under the header.
- **View:** a two-segment control for grid and list. Persist the choice per page in `localStorage` beside the existing sidebar key.
- Mirror search, sort and filter in the URL query so a view can be linked and restored.

### `TruncatedText`

Replace the always-on toggle with measurement.

- One-line contexts (names in cards, rows, table cells): CSS ellipsis plus a tooltip with the full text. No toggle.
- Multi-line contexts (descriptions): clamp, then compare `scrollHeight` with `clientHeight` in a `ResizeObserver`. Render the toggle only when clipped.
- Toggle label is "More" or "Less". Remove the per-field labels such as "Show full agent name".

### `EmptyState` and skeletons

- Every list has an empty state with an illustrated icon, one sentence explaining the concept, and a next step where one exists. The Agents empty state currently shows the wordmark in a box.
- Replace every "Loading…" paragraph with a skeleton that matches the final layout's dimensions, so nothing shifts when data arrives.

### Destructive actions

- Revoke and Disconnect are visible `destructive-outline` buttons, not menu items and not ghost text.
- An overflow menu is used only when it holds three or more secondary actions. A menu with one item is removed.
- Confirmation stays a dialog. It states the effect in one sentence, names the object, and focuses Cancel.
- After success the card animates out and a toast offers no undo unless the API supports one.

## Consent screen

Rebuild the consent screen as one 480 px card with three zones and a pinned footer, so the user reads who, what and how long, then decides, without scrolling.

Today `AgentDecisionPage` stacks up to ten full-width blocks in a 640 px column: wordmark, identity header with link buttons, localhost banner, a free-standing summary sentence, the permission accordion, three duration radio rows, a connection list, an "Advanced details" accordion, a next-steps paragraph, and the buttons at the very bottom. Each block has the same visual weight, so nothing leads the eye and the Allow button is off screen.

&#91;embedded content: consent card wireframe · 3 zones\]

Read the card top to bottom: identity and any risk signal first, the permission panel in the middle, and the duration and buttons in a footer that stays in view.

### Zone 1: who is asking

- **Shell:** the wordmark shrinks to an 18 px mark above the card. "Signed in as {name}" with a small avatar sits top right, so users on shared machines see which account is granting.
- **Title:** agent logo (40 px) with the agent name at 20 px semibold and "wants to act on your behalf" at 15 px regular in muted text.
- **Origin line:** muted text under the title. Use a success-coloured check for a verified origin and `CornerDownRight` for "Returns to".
- **About:** an info icon button beside the title opens a popover with the full description and the Governance, Documentation and Agent interface links. They are reference material and leave the main flow.
- **Risk callout:** shown only when there is a risk signal (localhost redirect, unverified origin). A soft warning surface with icon and one sentence: "This request returns to your own computer. Continue only if you started it from {Agent}." It sits directly under the origin line, where trust is being judged.

### Zone 2: what it gets

A visually separate inset panel (`--muted` background, 12 px radius) headed "It will be able to".

- **One row per permission group:** checkbox with both the name and description as labels, then small service icons on the right. Clamp descriptions at two lines; show a keyboard-operable "More" / "Less" text toggle only when measured as clipped. Expansion stays inside the permission panel. Do not use native title tooltips for permission text.
- **Required groups:** the checkbox is checked and locked, and a `Lock` icon with the word "Required" follows the name. Optional groups carry no label; an unlocked checkbox already says optional.
- **Already granted:** a muted check and "Granted" at the end of the description line, not a badge.
- **Services:** a separate right-side "2 of 3 services" control with a chevron opens selectable services. Clicking permission text toggles the group, not the service chooser.
- **Missing connection:** the service icon shows a warning dot and the row gets an inline `Connect` button. This removes the separate Connections section.
- **Overflow:** measure `scrollHeight > clientHeight`, not the number of groups. Show a bottom fade and a remaining-group hint such as "2 more" whenever content extends below the panel's visible edge. Recompute on scroll and resize. Keep the decision footer pinned.

### Zone 3: decide

- **Duration:** one line, "Access lasts" followed by a select with Until I revoke it, 30 days and Custom date. The date picker appears in a popover. This replaces three stacked radio rows.
- **Buttons:** Deny (outline) and Allow (primary) side by side, each half the card width, 40 px tall. Allow is on the right.
- **Blocked state (2026-10-06 correction):** keep the Allow label while selected services need connecting. Disable it with an explanation above the footer actions and leave inline Connect buttons available. Enable Allow only after all selected connections are available. This replaces the 2026-10-04 changing primary-action choice.
- **Reassurance:** one line of 12 px muted text under the buttons: "You can change or revoke this at any time in Agents." It replaces the next-steps paragraph.
- **Technical details:** a text link, right-aligned on the reassurance line, opens a dialog with client ID, redirect URI and requested scopes. The MCP security guidance expects the redirect URI to be available to the user ([MCP security best practices](https://modelcontextprotocol.io/specification/2025-06-18/basic/security_best_practices)), so also show the redirect host in the origin line when it differs from the agent's domain.

### States

| State | Behaviour |
| --- | --- |
| Loading | Skeleton with the card's final dimensions |
| Allowed | The Allow button turns into an animated check for 320 ms, then the redirect happens |
| Denied | Card content swaps to a neutral grey circle, "Access denied. Nothing was changed." and "You can close this tab." |
| Invalid session | Same card frame with an error icon, one sentence and no retry |
| Validation error | Inline under the affected row, and the footer shows one summary line |
| Below 640 px width | Card becomes full bleed. The footer is fixed to the bottom with a top border |

### Height budget

At 1280 × 720 the card must fit with three permission groups and a risk callout. Let the card grow up to `100dvh - 7rem` (`100dvh - 3.5rem` below 640 px), with no fixed 616 px card cap or 220 px permission-panel cap. The permission panel scrolls only when content exceeds the available viewport space.

### Review scope: 2026-10-05

C1–C3 and C5–C7 are included in the consent-screen correction. The user explicitly deferred C4 to a separate backend/API and security review. Keep off-origin agent images blocked; the current API has no service-logo URI. A future same-origin logo endpoint must validate image type and size, prevent SSRF, and bound its cache before it replaces the local fallback.


### Console variant

The same permission panel and duration control are reused on the agent detail page. Build them as one `PermissionPanel` and one `DurationSelect`, used by both `AgentDecisionPage` and the agent detail view.

## Agents view

Replace the three-column table with a card grid where the whole card opens the agent, and replace the tabbed detail page with a two-column layout.

### List (`DelegationsPage`, `DelegationsTable`)

| Your feedback | Change |
| --- | --- |
| Show more / less does nothing | Gone with the `TruncatedText` fix. Names truncate with a tooltip. |
| Expected a click on the agent to open details | The whole `EntityCard` is a link to `/agents/:id`. The separate View button is removed. |
| A table is overkill for fewer than ten agents | Card grid by default, list through the view toggle. |
| Layout is unstructured | Cards sit on the 12-column grid, three per row at 1120 px, two at 768 px, one below. |

Each agent card shows:

- **Leading:** logo or tinted initial, 40 px rounded square.
- **Title:** agent name.
- **Supporting line:** access duration in words, "Until revoked" or "Expires 12 Nov". Use a warning tint when expiry is within seven days.
- **Meta:** "Changed 3 days ago" from `lastModifiedAt`, which the API already returns and the table ignores.
- **Footer action:** `Revoke` as a small `destructive-outline` button. It is the only action, so it needs no menu.

The `activeGrantCount` field stays hidden, as `AGENTS.md` requires. The full-width search input above the table is replaced by the toolbar's search icon.

### Detail (`AgentConsolePage`)

The current page stacks the identity header, a row of outline link buttons, a pill tab list and an accordion. The link buttons and the tabs are the same height and style, which is why they clash.

New layout on the 8 + 4 grid:

- **Header row:** back link to Agents, logo, name, origin line. `Revoke access` sits on the right as a `destructive-outline` button. The three-dot menu is removed because it held one item.
- **Description:** full text under the header, capped at `70ch`, with the measured "More" toggle only beyond three lines.
- **Main column (8):** the `PermissionPanel` from the consent screen, in its editable console mode, followed by `DurationSelect`.
- **Side rail (4):** two quiet sections. "Connections" lists the agent's services as compact rows with status and a `Connect` or `Manage` action. "About" lists Governance, Documentation and Agent interface as text links with an external-link icon, and shows only the links that are configured.
- **No tabs.** With a handful of services, Connections fits in the rail. This removes the tab-versus-button conflict instead of restyling it.
- **Save bar:** keep the sticky `GrantEditBar`; it is a good pattern. Give it the raised surface colour and a top shadow so it reads as floating.

Below 1012 px the side rail moves under the main column.

### Permission rows

- The chevron sits directly after the group name, not at the far right edge.
- The whole row header toggles the service list. The checkbox remains a separate target.
- Groups with three or fewer services show them expanded by default in the console, since there is room.
- "Required" becomes a lock icon and the word beside the name. "Optional" and "Already granted" badges are removed.

## Connections view

Connections uses the same card grid as Agents, with the connection state as the single badge and one contextual action.

| Your feedback | Cause | Change |
| --- | --- | --- |
| Show more / less does nothing | `TruncatedText` on the provider name and on the notice banner | Tooltip truncation for names. Notices move to toasts. |
| Clicking highlights the whole box | Page wrapper in `ConnectionsPage.tsx` has `tabIndex={-1}` and `focus:ring-2` | Use `focus-visible`, or move programmatic focus to the page heading. |
| A table is overkill | Five columns, two of them a count and a timestamp | `EntityCard` grid, list through the toggle. |

Each connection card shows:

- **Leading:** provider logo or tinted initial.
- **Title and badge:** provider name, then the state badge (Connected, Needs sign-in, Expired, Unavailable).
- **Supporting line:** "4 scopes · connected 12 Sep". The scope count is a button that opens a popover listing the scopes in the mono face. The API already returns the scope array; the table only shows its length.
- **State explanation:** when the state is not Connected, one line of muted text replaces the supporting line, for example "Your access expired. Refresh to continue." Today this text sits under the badge inside a table cell and changes the row height.
- **Footer actions:** the contextual action (`Reconnect`, `Refresh` or `Retry`) as an `outline` button, and `Disconnect` as `destructive-outline`. A connected card shows only Disconnect.

Further changes:

- **Filter:** the toolbar's filter popover offers connection state. Cards that need attention sort first by default.
- **Feedback:** the inline notice bar with its own Close button and a five-second timer is replaced by the existing Sonner toasts, which Agents already uses.
- **Callback result:** after returning from a provider, the new or refreshed card pulses its border in the accent once (320 ms) and the animated plug icon plays.
- **Empty state:** icon, "No connections yet", and the sentence "You connect a service when an agent asks for access to it."
- **Disconnect dialog:** keep the list of dependent agents. It is the most useful part of the current flow.

## Approvals view

Turn Approvals into an inbox: a stable list of pending requests on the left and one detail panel on the right where the decision is made. Remembered decisions move to a second tab.

The controls you found fine stay. What changes is where they live. Today `InlineApprovalActions` renders the persistence radio cards, scope preview and scope editor inside the Actions cell of every row, and each click adds or removes blocks. The page then stacks two more tables, "Standing allow decisions" and "Standing deny decisions", below. A second, cleaner implementation of the same decision already exists in `ApprovalReviewPage` for `/approvals/:id`.

&#91;embedded content: approvals layout wireframe · list and detail panel\]

Selecting a row fills the panel on the right. The caret on each button holds the remembered options, so choosing one never changes the list's height.

### Pending tab

- **List (5 columns):** fixed-height `EntityRow` per request with the agent avatar, the tool name, the agent name as supporting text, a risk badge and the age. A thin progress line or "expires in 4 min" shows the time left. Rows never expand.
- **Detail panel (7 columns):** sticky beside the list. It shows the selected request using the content of `ApprovalReviewPage`: what the agent wants to do, the arguments, and what the decision covers.
- **Arguments:** show the top-level keys and values as a two-column list when there are six or fewer, with "View JSON" opening the raw payload in a dialog. MCP guidance says tool inputs should be shown to the user before the call ([MCP tools](https://modelcontextprotocol.io/specification/2025-06-18/server/tools)), so they should not start collapsed.
- **Decision bar:** pinned to the panel's bottom edge. `Deny` (outline) on the left, `Approve once` (primary) on the right. Each has a caret menu: Approve offers "For this session" and "Always…", Deny offers "Always deny…".
- **Remembered approvals:** choosing "Always…" swaps the panel body to the scope editor with Back and Confirm. The list does not move.
- **After a decision:** the row slides out, a toast confirms, and the next request is selected automatically.
- **Keyboard:** `J` and `K` move through the list, `A` approves once, `D` denies, `Enter` focuses the panel. Show the shortcuts in button tooltips. Linear's inbox and triage use the same model ([Inbox](https://linear.app/docs/inbox), [Triage](https://linear.app/docs/triage)).
- **Below 1012 px:** the list fills the page and a row opens the existing `/approvals/:id` route.
- **Empty state:** an animated shield-check, "You're all caught up", and one line explaining that requests appear here when an agent needs a decision.

Delete `InlineApprovalActions` and `PendingApprovalsTable`. The panel and the standalone review page then share one implementation.

### Remembered tab

- **Naming:** "Standing allow decisions" and "Standing deny decisions" become one tab called Remembered, with a filter for All, Always allowed and Always denied.
- **Rows:** tool name, agent, a soft badge (Always allowed or Always denied), the scope pattern on one truncated mono line with the full pattern in a popover, and a `Revoke` button in the `outline` variant.
- **Tabs:** underline tabs in the page header, "Pending · 2" and "Remembered", mapped to `/approvals` and `/approvals/remembered`.

### Risk

- Map risk to the soft badge colours: low green, medium amber, high and critical red, with critical adding a filled icon.
- For high and critical requests, tint the detail panel's header with the soft danger surface. This is the "critical information arrives" case and deserves more than a badge.
- "Risk not rated" becomes neutral grey with a dash icon.

## Settings

Settings leaves the sidebar and opens from the user menu as a page with one view per category.

The current page is a single card holding the theme choice, which the user menu already offers. A dedicated navigation item for one duplicated control is overhead.

- **Entry:** "Settings" in the user menu, plus a command-palette entry. Remove the sidebar item.
- **Layout:** a `SettingsLayout` with a vertical category list on the left (3 columns) and the category view on the right (9 columns). Each category has its own route, starting with `/settings/appearance`.
- **Content style:** plain setting rows, not cards. Label and one-line description on the left, the control on the right, a `--border-subtle` divider between rows.
- **Appearance:** show the theme choice as three small preview tiles (Light, Dark, System) instead of a radio list. Add the default collection view (grid or list) here, since it is now a stored preference.
- **One category today:** hide the category list while only Appearance exists, and show it as soon as a second category is added. The layout and routes are in place, and the page does not show a one-item menu.

A change applies immediately and is confirmed by the control itself. No save button and no toast.

## Storybook and quality gates

The Storybook setup is current, but it only covers primitives. All 34 stories live under `design-system/`. No story exists for a composed component or a page, which is where every problem in this review sits.

What is already right and should stay: Storybook 10 with the Vitest addon, the theme toolbar through `withThemeByDataAttribute`, four viewports, and `a11y.test: 'error'` so accessibility violations fail the run.

### Add three story layers

| Layer | Examples | Purpose |
| --- | --- | --- |
| Foundations (MDX) | Colour roles with contrast values, type scale, spacing, radii, motion, icon map | The visual contract, rendered from the tokens instead of described in Markdown |
| Patterns | `EntityCard`, `EntityRow`, `CollectionToolbar`, `PermissionPanel`, `DurationSelect`, approval detail panel, `EmptyState` | Each state as its own story |
| Screens | Consent, Agents, Agent detail, Connections, Approvals, Settings | Whole pages in the real shell, per state and viewport |

### Make screens storyable without a network

Split each page into a container that calls hooks and a pure `*View` component that takes data and callbacks as props. Stories render the view with fixtures from one shared `fixtures/` module. This needs no mocking library and matches the existing rule that shells fetch no data.

### Required stories per screen

- Loading, empty, typical, dense (ten or more items), error and stale.
- Longest realistic content: an 80-character agent name, a 400-character description, eight permission groups.
- Consent adds: risk callout, missing connection, already granted, denied outcome, invalid session.
- Approvals adds: high-risk selected, expired request, remembered-scope editor open.
- Each at 375, 768 and 1280 px, in light and dark.

### Gates

- **Interaction tests:** a `play` function for each key flow (toggle a permission, pick a duration, approve from the keyboard, revoke with confirmation). They run in the existing `storybook-light` and `storybook-dark` Vitest projects ([Storybook Vitest addon](https://storybook.js.org/docs/writing-tests/integrations/vitest-addon)).
- **Layout stability:** in each `play` test, record the bounding box of the list before and after an interaction and assert that it is unchanged. This encodes principle 3.
- **Viewport fit:** for the consent typical story at 1280 × 720, assert that the Allow button's bounding box is inside the viewport.
- **Visual regression, amended 2026-10-06:** [ADR 040](../../adrs/040-canonical-light-visual-reference-gate.md) requires reviewed light 1280 × 720 references for eligible pattern and screen cases. Capture and comparison require desktop `storybook-light`, effective light theme, and the canonical actual viewport. All six projects retain interaction and accessibility checks. Intentional dark or narrow overrides receive no PNG reference.
- **Contrast:** extend `contrast.test.ts` to every foreground and background pair in the new token table, including soft status pairs.
- **End-to-end:** the Ginkgo and Playwright journeys in `tests/e2e/frontend` use the page objects in tests/e2e/pages, which select by role, label and `data-testid`. Keep the existing IDs and accessible names where the element survives, and update the journeys in the same change where it does not (the tables, the inline approval actions).

## Work packages

Eleven packages, in dependency order. Packages 0 to 3 are sequential. Packages 4 to 8 can then run in parallel, one agent each. Hand each agent its package row, the matching section above and the design principles.

| # | Package | Scope | Done when |
| --- | --- | --- | --- |
| 0 | ADR 038 and spec update | Amend ADR 038 in place: palette, border roles, shell and page layouts, Motion on console routes, and the five approved decisions below. Update the feature 047 spec, plan and tasks, plus `DESIGN_PRINCIPLES.md`, `COLOR_GUIDE.md`, `TOKEN_GUIDE.md`, `MOTION_GUIDE.md`, `web/AGENTS.md` (the line fixing the Agents table columns) and `contracts/ui-and-configuration.md`. | ADR 038 and the 047 spec describe the new direction. No guide contradicts this plan. |
| 1 | Tokens and primitives | `theme.css` values below. Three border roles. `Badge`, `Button`, `Alert`, `Avatar` changes. Cropped logo SVGs. Motion tokens and reduced-motion rule. | `contrast.test.ts` passes for all pairs. No component uses `--border-control` except form controls. Primitive stories pass both-theme accessibility and use reviewed canonical light references under ADR 040. |
| 2 | `TruncatedText` and focus fixes | Measured truncation. Tooltip for one-line text. `focus-visible` on programmatic focus targets. | No toggle renders for text that fits (interaction test). Clicking page background shows no ring. |
| 3 | Shell and shared patterns | `ConsoleShell` changes, centred container and grid, `PageHeader`, `EntityCard`, `EntityRow`, `CollectionToolbar`, `EmptyState`, skeletons, underline tabs. Route rename to /agents and /connections, including the provider callback URL. | Pattern stories exist for every state. Sidebar has three items and an icon collapse control. |
| 4 | Consent screen | `DecisionShell`, `AgentDecisionPage`, `PermissionPanel`, `DurationSelect`, About popover, technical-details dialog, outcome states. | Allow is inside the viewport at 1280 × 720 with three groups and a risk callout. No raw paths in copy. All consent journeys pass. |
| 5 | Agents | Card grid, list toggle, agent detail on the 8 + 4 grid without tabs. Remove `DelegationsTable` and `AgentOverflowMenu`. | Whole card navigates. Revoke is a visible button. Chevron sits beside the group name. |
| 6 | Connections | Card grid, scope popover, state filter, toasts instead of the notice bar, unified "connection" wording. Remove `ConnectionsTable`. | No row or card changes height on any state. Copy contains no "session" in user-facing strings. |
| 7 | Approvals | List and detail panel, split buttons, keyboard shortcuts, Remembered tab. Remove `InlineApprovalActions`, `PendingApprovalsTable`, the two standing tables. | List bounding box is unchanged by any decision step (interaction test). Panel and `/approvals/:id` share one component. |
| 8 | Settings | `SettingsLayout`, `/settings/appearance`, setting rows, theme tiles, entry in the user menu. | No Settings item in the sidebar. Theme and default view persist. |
| 9 | Icons and motion | Icon map applied. Eight animated icons: lucide-animated with Motion on console routes, owned CSS on decision routes. View transition for card to detail. Motion for card and row removal. | Animations stop under reduced motion. No decision route imports Motion (bundle test). |
| 10 | Storybook and gates | Foundations MDX, screen stories from `*View` components and shared fixtures, interaction and viewport-fit tests. | Every screen has the required state stories at three widths in both themes, all passing accessibility checks. |

### Light theme values for package 1

| Token | Current light | Proposed light |
| --- | --- | --- |
| `--background` | `oklch(0.985 0 0)` | `oklch(0.985 0.003 260)` |
| `--muted` | `oklch(0.95 0 0)` | `oklch(0.965 0.005 260)` |
| `--foreground` | `oklch(0.20 0 0)` | `oklch(0.21 0.012 260)` |
| `--muted-foreground` | `oklch(0.45 0 0)` | `oklch(0.47 0.012 260)` |
| `--border-subtle` | `oklch(0.90 0 0)` as `--border-soft` | `oklch(0.93 0.006 260)` |
| `--border` | `oklch(0.62 0 0)` | `oklch(0.88 0.008 260)` |
| `--border-control` | uses `--border` | `oklch(0.60 0.012 260)` (3.8:1 on background) |
| `--primary` | `oklch(0.48 0.14 255)` | `oklch(0.50 0.19 262)` |
| `--primary-soft` | none | `oklch(0.95 0.03 262)` with text `oklch(0.42 0.17 262)` |
| Status soft | solid `oklch(0.45 c h)` | background `oklch(0.95 0.04 h)`, text `oklch(0.42 0.11 h)` (7.0–7.5:1) |

Status hues stay at 150 (success), 75 (warning) and 25 (danger).

### Approved decisions

All five were approved on 4 October 2026. Package 0 records them in ADR 038 and the 047 spec.

- [x] Cards by default for Agents and Connections, with a list view through the toggle.
- [x] Motion is allowed on console routes. Decision routes stay CSS-only.
- [x] On consent, the primary button becomes "Connect {Service} to continue" when a connection is missing. `DESIGN_PRINCIPLES.md` changes its rule that the only accent action on consent is Allow.
- [x] `/delegations` and `/sessions` are renamed to `/agents` and `/connections`.
- [x] The consent screen shows "Signed in as".

## Sources

Code references are to `zalando-incubator/agentic-identity-broker`, branch `047-redesign-consent-console`, commit `2e6755b`. External pages were opened on 4 October 2026. The Claude artifact gallery reference comes from your two screenshots.

| Topic | Source | Used for |
| --- | --- | --- |
| Consent | [MCP security best practices](https://modelcontextprotocol.io/specification/2025-06-18/basic/security_best_practices) | Client name, scopes and redirect URI on the consent page |
| Consent | [Microsoft Entra consent experience](https://learn.microsoft.com/en-us/entra/identity-platform/application-consent-experience) | Order of the identity header, signed-in user, verified publisher line |
| Consent | [Google granular permissions](https://developers.google.com/identity/protocols/oauth2/resources/granular-permissions) | Per-permission checkboxes |
| Consent | [NN/g permission requests](https://www.nngroup.com/articles/permission-requests/) | Sentence-style request and the link to where a grant can be reversed |
| Consent | [PoPETs 2017, Look Before You Authorize](https://petsymposium.org/popets/2017/popets-2017-0014.php) | Habituation; highlight the few risky facts |
| Approvals | [MCP tools specification](https://modelcontextprotocol.io/specification/2025-06-18/server/tools) | Show tool inputs before a call |
| Approvals | [Linear Inbox](https://linear.app/docs/inbox), [Linear Triage](https://linear.app/docs/triage) | List and detail layout, single-key decisions |
| Approvals | [LangChain human-in-the-loop](https://docs.langchain.com/oss/python/langchain/human-in-the-loop) | Approve, edit and reject as the decision set |
| Collections | [NN/g cards](https://www.nngroup.com/articles/cards-component/) | Cards scan more slowly than lists |
| Collections | [Geist Entity](https://vercel.com/geist/entity) | Row anatomy, at most two visible actions |
| Collections | [Polaris resource list](https://polaris-react.shopify.com/components/lists/resource-list) | List instead of table for find-and-act |
| Collections | [Linear display options](https://linear.app/docs/display-options), [filters](https://linear.app/docs/filters) | Compact toolbar, filters as chips and in the URL |
| Navigation | [Primer navigation patterns](https://primer.style/product/ui-patterns/navigation/) | Tabs change the URL, actions are not navigation, segmented control for grid and list |
| Navigation | [shadcn/ui Sidebar](https://ui.shadcn.com/docs/components/sidebar) | Icon collapse trigger, user menu in the footer |
| Colour | [Radix Colors scale](https://www.radix-ui.com/colors/docs/palette-composition/understanding-the-scale) | Separate steps for surfaces, borders and text |
| Colour | [Geist colours](https://vercel.com/geist/colors) | Borders quieter than solids |
| Colour | [Linear UI redesign](https://linear.app/blog/how-we-redesigned-the-linear-ui) | Themes generated in a perceptual colour space |
| Colour | [web.dev colour scheme](https://web.dev/articles/building/a-color-scheme) | Dark surfaces separated by lightness, reduced saturation |
| Badges | [Geist badge](https://vercel.com/geist/badge), [Atlassian lozenge](https://atlassian.design/components/lozenge/usage), [Carbon tag](https://carbondesignsystem.com/components/tag/usage/) | One badge per row, subtle variant, baseline alignment |
| Motion | [NN/g animation duration](https://www.nngroup.com/articles/animation-duration/) | 100 ms feedback, 200–300 ms overlays, exits shorter |
| Motion | [lucide-animated](https://lucide-animated.com) | Ready-made animated Lucide icons |
| Motion | [MDN View Transition API](https://developer.mozilla.org/en-US/docs/Web/API/View_Transition_API) | Card-to-detail transition without a library |
| Layout | [Atlassian spacing](https://atlassian.design/foundations/spacing), [Primer layout](https://primer.style/product/getting-started/foundations/layout/) | Spacing steps, breakpoints, narrow interstitial pages |
| Layout | [Baymard line length](https://baymard.com/blog/line-length-readability), [Geist typography](https://vercel.com/geist/typography) | 70-character measure, 14 px base |
| Storybook | [Vitest addon](https://storybook.js.org/docs/writing-tests/integrations/vitest-addon), [interaction testing](https://storybook.js.org/docs/writing-tests/interaction-testing), [accessibility testing](https://storybook.js.org/docs/writing-tests/accessibility-testing) | Stories as tests, `play` functions, failing on violations |

Not verified: Apple's and Material's dark-mode pages did not load, so the dark theme guidance rests on Radix, Geist, Linear and web.dev. Stripe, Okta and WorkOS consent screens were not reviewed.
