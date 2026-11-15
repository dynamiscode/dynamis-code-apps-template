---
name: Dynamis Code Apps Template
description: An accessible, task-focused foundation for generated web applications.
colors:
  ink: "#172033"
  muted: "#536174"
  surface: "#fff"
  canvas: "#f3f6fa"
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
  navigation-label:
    fontFamily: "system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 800
    letterSpacing: "0.06em"
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

The implemented interface puts workspace tasks before visual decoration. Cool
slate text and backgrounds frame a deep blue accent. Controls have explicit
borders and visible states; content cards stay flat. The template provides a
neutral starting point for generated applications, which own their branding.

**Key Characteristics:**

- System fonts and a compact, readable text hierarchy.
- Light and dark themes with semantic colors.
- Bordered controls and cards, with shadows on floating menus and toasts.
- Responsive navigation, ordinary forms, and visible keyboard focus.

`internal/web/assets/app.css` is the source of truth. This document records
the current implementation, not a proposed redesign. The sidecar's tonal
ramps are generated previews, not implemented color tokens.

## Colors

The light palette pairs deep blue actions with cool slate text and pale
backgrounds. Dark mode replaces the same semantic roles rather than inverting
the light palette.

### Primary

- **Deep blue:** `accent` colors links and primary controls; `accent-hover`
  marks hover. `accent-surface` identifies selected navigation.
- **On accent:** `on-accent` supplies text for primary controls.

### Feedback

Danger, warning, and success each have a foreground and a pale background.
Warning also has a separate border color. These colors mark alerts and feedback,
not interchangeable decorative accents.

### Neutral

`ink` and `muted` distinguish primary and supporting text. `canvas` surrounds
the white `surface`; border tokens distinguish cards, form controls, and
disabled controls. Each role has a corresponding `dark-` token in the
frontmatter. The stylesheet also defines theme-specific shadow colors.

The account preference selects light, dark, or system appearance. System
appearance follows `prefers-color-scheme`; see [Theme](docs/web.md#theme).

## Typography

The base uses the system UI font stack at the browser's default size and a
1.5 line height. Labels and controls use bold weight; navigation group labels
use uppercase, wider letter spacing, and the `navigation-label` style.
Headings retain browser defaults rather than a separately defined type scale.

## Layout

The authenticated workspace shell is centered and capped at `80rem`. Its
desktop grid pairs a `15rem` sidebar with flexible content. Main content is
capped at `64rem`; narrow pages such as sign-in use `32rem`. Main horizontal
padding uses `clamp(1rem, 3vw, 2rem)`.

At `38rem` and below, the top bar wraps, the sidebar moves above the content,
and rows and actions stack. Tables use a horizontal scroll wrapper. The
accessibility checks cover reflow at a 320 CSS-pixel viewport.

## Elevation & Depth

Cards and controls are flat, separated by borders and background color.
Workspace and account menus use `0 .5rem 1.5rem var(--shadow)`; toasts use
`0 .25rem 1rem var(--shadow)`. The shadow color changes with the theme.

## Shapes

Controls and navigation links use a `0.25rem` radius. Cards and floating menu
panels use `0.4rem`. Controls have a `2px` border; cards use a `1px` border.
Status messages use a prominent left border.

## Components

### Buttons and fields

Buttons and inputs have a `2.75rem` minimum height and `.45rem .65rem`
padding. Primary buttons use the accent; button-styled links use a darker hover
state. Secondary buttons use the surface and accent. Disabled buttons use
muted text, a muted border, and a disabled surface. Fields span their container
and use the strong border; invalid fields switch to the danger border.

Keyboard focus uses a `.2rem` outline plus a `.4rem` outer halo. Preserve both
layers on light and dark backgrounds. The stylesheet removes animation and
transitions when reduced motion is requested.

### Cards and feedback

Workspace links and item forms use bordered surface cards with `1rem` padding
and `.5rem` internal gaps. Errors, warnings, and success messages pair color
with text and a left border. Toasts use a floating bordered surface and shadow.

### Navigation

The top bar contains brand, workspace context, and account controls. Workspace
and account menus use native disclosure elements. The workspace sidebar marks
the current link with bold text and the accent surface; navigation links retain
a `2.75rem` minimum height. At the narrow breakpoint, the sidebar stacks above
the content.

## Do's and Don'ts

### Do:

- **Do** use the semantic theme tokens for both light and dark appearances.
- **Do** retain visible keyboard focus, labeled controls, and ordinary form
  fallback when adapting the template.
- **Do** check narrow-screen reflow and reduced-motion behavior.

### Don't:

- **Don't** convey error, warning, success, or selection with color alone.
- **Don't** treat template branding as mandatory for generated applications.
