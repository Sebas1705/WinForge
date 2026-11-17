# UI audit, 2026-11-17

How the interface was checked, what was found and what was changed. The method can be repeated.

## Method

- Edge (headless) driven by Playwright, with axe-core 4 (`wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa`, `wcag22aa`, best practices) on every page and dialog.
- The Go API was replaced by a stub with realistic data, so every page shows content (lists, results, findings).
- Pages: Home, Profiles (list, compare, offline kit), Catalog, Updates, PC health, Storage (cleanup, projects, disks, map), Startup (programs, services, tasks), Developer, Privacy, Uninstall, Games, Settings backup, app detail, Settings and the palette.
- Matrix: 6 styles x light and dark at 1100 px, and 4 style/theme pairs at 640 px, in English and in Spanish (320 audits per language).
- Own checks on top of axe: the document must not scroll, nothing wider than the page, touch targets of at least 24 px, and text contrast of every visible text node.
- Full-page screenshots at 1280 px were read by eye.

## Found and fixed

- **The whole window could scroll on Home** (document height 1019 px in a 760 px window): absolutely positioned screen-reader text and ring SVGs inside a page that was not positioned extended the document. `html, body` do not scroll now and pages are the containing block of what is positioned in them.
- **Label in name (WCAG 2.5.3):** app cards, "Go back to a version", Health area buttons, "Restore original" buttons and space-map blocks had an accessible name that did not contain their visible text. Names now start with, and contain, what is written on the control.
- **Scrollable tables could not be reached with the keyboard** (WCAG 2.1.1): every scrollable table is now focusable and named.
- **Heading order:** dialog titles are `h2` instead of `h3`.
- **Invalid role:** the app detail drawer was an `aside` with `role="dialog"`; it is a `div` now.
- **Touch targets:** checkboxes and radios are 18 px inside labels of at least 28 px; the card checkbox is 28 px; a checkbox outside a label is 24 px.
- **Narrow windows:** the catalog toolbar and footer, and hint rows, wrap instead of running past the edge; Startup and disk lists shrink instead of widening the page.
- **Search boxes** looked unlike other inputs; they share the same style now.
- **Texts such as "1 apps"** were rewritten so they do not depend on the number.

## Result after the fixes

axe reports no violations on any page in either language. One non-actionable block of the space map ("Other") is smaller than 24 px by design: it is marked `aria-disabled`.

## Not covered

- axe could not decide `color-contrast` on the progress rings and on the sidebar brand in a few style and theme pairs (backgrounds with gradients or glass); the own text-contrast check passed there, but a human should look.
- Glass over glass in the liquid style (two stacked dialogs) is measured against the page, not against the dialog below.
- No screen reader was used and keyboard order was not scripted; only roles, names and focus targets were checked.
- The Go side was stubbed, so layout with real data lengths (long paths, many items) is only as good as the stub.
