---
name: Dynamis Code Apps Template
description: An accessible, task-focused foundation for generated web applications.
colors:
  ink: "#172033"
  muted: "#536174"
  surface: "#fff"
  canvas: "#f3f6fa"
  sidebar-surface: "#edf1f7"
  accent: "#154ea3"
  accent-hover: "#103d80"
  accent-surface: "#e8effa"
  on-accent: "#fff"
  danger: "#a51d2d"
  danger-surface: "#fff0f1"
  warning: "#8a5a00"
  warning-surface: "#fff8db"
  warning-border: "#d6b656"
  success: "#176b3a"
  success-surface: "#edf8f0"
  border: "#ccd3dd"
  border-strong: "#657287"
  border-muted: "#aeb7c5"
  disabled-surface: "#e2e6ec"
  focus-outline: "#fff"
  focus-halo: "#111827"
  dark-ink: "#e6ebf2"
  dark-muted: "#a4afbf"
  dark-surface: "#1a2231"
  dark-canvas: "#111826"
  dark-sidebar-surface: "#151d2a"
  dark-accent: "#8ab4f8"
  dark-accent-hover: "#aecbfa"
  dark-accent-surface: "#1f2d47"
  dark-on-accent: "#0b1220"
  dark-danger: "#ff8a8a"
  dark-danger-surface: "#3a1a1f"
  dark-warning: "#f0c36b"
  dark-warning-surface: "#3a2f10"
  dark-warning-border: "#7a6226"
  dark-success: "#7fd69a"
  dark-success-surface: "#15301f"
  dark-border: "#34405a"
  dark-border-strong: "#7d8aa0"
  dark-border-muted: "#4a5670"
  dark-disabled-surface: "#2a3344"
  dark-focus-outline: "#e6ebf2"
  dark-focus-halo: "#0b1220"
typography:
  body:
    fontFamily: "system-ui, sans-serif"
    fontSize: "100%"
    lineHeight: 1.5
  page-heading:
    fontFamily: "system-ui, sans-serif"
    fontSize: "2rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.02em"
  section-heading:
    fontFamily: "system-ui, sans-serif"
    fontSize: "1.375rem"
    fontWeight: 700
    lineHeight: 1.3
  navigation-label:
    fontFamily: "system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 800
    letterSpacing: "0.06em"
  supporting-text:
    fontFamily: "system-ui, sans-serif"
    fontSize: "0.875rem"
    color: "{colors.muted}"
rounded:
  control: "0.25rem"
  card: "0.4rem"
spacing:
  half: "0.5rem"
  compact: "0.75rem"
  standard: "1rem"
  section: "1.5rem"
  wide: "2rem"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.control}"
    padding: "0.45rem 0.65rem"
  button-link-hover:
    backgroundColor: "{colors.accent-hover}"
    textColor: "{colors.on-accent}"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.accent}"
    rounded: "{rounded.control}"
    padding: "0.45rem 0.65rem"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "0.45rem 0.65rem"
  card:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.card}"
    padding: "1rem"
---

# Design System: Dynamis Code Apps Template

## Overview

**Creative North Star: "Clear workspace"**

The system is calm and task-first: cool slate surfaces frame deep blue actions,
while compact controls and clear selected states keep workspace work prominent.
Borders carry structure; cards stay flat, with elevation shadows reserved for
floating menus and toasts. The template stays visually neutral so generated
applications can establish their own identity.

**Key Characteristics:**

- System fonts and a compact, readable text hierarchy.
- Light and dark themes with semantic colors.
- Explicitly bordered controls and cards; elevation shadows on floaters, plus a separate keyboard-focus halo.
- Responsive navigation, ordinary forms, and visible keyboard focus.

`internal/web/assets/app.css` is the source of truth. This document records
the current implementation, not a proposed redesign. Tonal ramps in the
sidecar are generated previews, not implemented color tokens.

## Colors

The light palette pairs deep blue actions with cool slate text and pale
backgrounds; dark mode reassigns the same semantic roles with darker surfaces
and lighter text and accents.

### Primary

- **Deep blue:** `accent` colors links and primary controls; `accent-hover`
  marks hover, while `accent-surface` identifies selected navigation.
- **On accent:** `on-accent` supplies text for primary controls.

### Feedback

Danger, warning, and success each have a foreground and a pale background;
warning also has a separate border color. These semantic colors identify
feedback, not interchangeable decoration.

**The Semantic-State Rule.** Pair state color with text or a distinct border;
never use color alone to convey error, warning, success, or selection.

### Neutral

`ink` and `muted` distinguish primary and supporting text. `canvas` surrounds
the `surface`; `sidebar-surface` separates navigation from workspace content.
Borders distinguish cards, form controls, and disabled controls. The frontmatter
records each role's light and dark values. Shadow color also changes with the theme.

The account preference selects light, dark, or system appearance. System
appearance follows `prefers-color-scheme`; see [Theme](docs/web.md#theme).

## Typography

The base uses the system UI font stack at the browser's default size with a
`1.5` line height. Labels and controls use bold weight. Navigation group labels
use uppercase text, a `0.75rem` size, `800` weight, and `0.06em` letter
spacing. Workspace switcher metadata uses muted supporting text at `0.875rem`.
Page headings use a fixed scale: `2rem` / `1.2` for `h1` and `1.375rem` / `1.3`
for visible `h2` headings.

## Layout

The top bar and authenticated workspace shell align to an `80rem` maximum.
The desktop shell pairs a `15rem` sidebar with flexible content. Main content
is capped at `64rem`; narrow pages such as sign-in use `32rem`. Main horizontal
padding uses `clamp(1rem, 3vw, 2rem)`.

At `38rem` and below, the top bar wraps and the sidebar moves above the content.
Workspace and settings navigation use two columns; rows and actions stack.
Narrow pages use bordered surface panels. Tables scroll horizontally when
needed. Preserve reflow at a `320px` CSS viewport.

## Elevation & Depth

Cards and controls stay flat, separated by borders and background color.
Workspace and account menus use `0 .5rem 1.5rem var(--shadow)`; toasts use
`0 .25rem 1rem var(--shadow)`. The keyboard-focus halo is a separate
interaction indicator, not surface elevation.

**The Float-Only Elevation Rule.** Reserve elevation shadows for floating menus
and toasts; keep the focus halo for keyboard focus, not surface depth.

## Shapes

Controls and navigation links use compact `0.25rem` corners. Cards and floating
menu panels use `0.4rem` corners. Buttons and fields use `2px` borders; cards
use `1px` borders. Status messages use `1px` semantic borders and matching
backgrounds.

## Components

### Buttons

Primary buttons use the accent; button-styled links darken on hover. Secondary
buttons use the surface and accent. Destructive actions use the danger color.
Buttons have a `2.75rem` minimum height and `.45rem .65rem` padding. Disabled
buttons use muted text, a muted border, and a disabled surface.

### Inputs / Fields

Fields fill their container, use the surface background and strong border, and
share the `2.75rem` minimum height and `.45rem .65rem` padding. Invalid fields
switch to the danger border. Labels are bold and remain visible.

### Cards / Containers

Workspace links use a surface background, `1px` border, `0.4rem` radius, and
`1rem` padding. Item records group edit and share controls in one bordered
surface. Narrow pages use a bordered surface panel.

### Feedback

Errors, warnings, and success messages pair semantic color with text and a
`1px` border and matching background. Toasts use a floating bordered surface
and shadow.

### Navigation

The top bar contains brand, workspace context, and account controls. Workspace
and account menus use native disclosure elements. The sidebar marks the current
link with bold text and the accent surface. Navigation links retain a `2.75rem`
minimum height. At the narrow breakpoint, workspace and settings navigation
use two columns above the content. The workspace switcher panel stays within
the viewport.

### Focus

Keyboard focus uses a `.2rem` outline plus a `.4rem` outer halo. Preserve both
layers in light and dark themes. The stylesheet removes animation and
transitions when reduced motion is requested.

### Browser surfaces

Links use `.15em` underline offset and `.08em` thickness. Text selection,
carets, and scrollbars use semantic theme colors; table numerals are tabular.

## Do's and Don'ts

### Do:

- **Do** use semantic theme colors in both light and dark appearances.
- **Do** preserve visible keyboard focus, persistent labels, and ordinary form
  fallback.
- **Do** check narrow-screen reflow and reduced-motion behavior.

### Don't:

- **Don't** convey error, warning, success, or selection with color alone.
- **Don't** add decorative elevation to static cards or controls.
- **Don't** treat template branding as mandatory for generated applications.
