---
name: ux-workflow
description: Design, change, or evaluate browser user journeys, navigation, layout, components, forms, interaction states, or visual hierarchy. Use for new pages and meaningful UX/UI changes; inspect the rendered journey and justify consequential pattern choices.
---

# UX workflow

Follow [UX standard](../../../docs/ux.md), [decision guide](../../../docs/ux-decisions.md), and [browser review](../../../docs/ux-review.md). Existing code is evidence, not proof of usability. Work within the repository's architecture, accessibility, authorization, and progressive-enhancement contracts.

## Before implementation

1. Identify user, task, context, entry and return paths, desired outcome, roles, and constraints. Trace existing behavior and related journeys. Use realistic data, including multiple similar records.
2. Inventory screens, actions, and applicable states. Identify the main decision and next action at each step. Write observable acceptance criteria, including failure and recovery.
3. Read [journeys and structure](references/journeys.md) for navigation, hierarchy, layout, and content; read [interactions and states](references/interactions.md) for controls, editing, feedback, and safety. Apply both to affected work.
4. For consequential choices, read [pattern selection](references/patterns.md) and compare at least two options against user task, complexity, accessibility, and existing architecture. Research authoritative guidance when patterns are missing, questionable, unfamiliar, or complex. Record choice, trade-offs, evidence, and uncertainty. Do not assume the existing pattern is best.
5. Select appropriate components and states. Reuse suitable existing components; repair or replace weak patterns rather than repeating them. Ask only when a consequential product decision cannot be inferred.

## Review and completion

1. Apply [evaluation](references/evaluation.md) from three perspectives: journey, interaction and visual hierarchy, and accessibility and implementation. When authorized, independent reviewers can own separate perspectives; otherwise record three distinct passes and state they are not independent.
2. Drive each affected journey in the real application. Review desktop and narrow screens and risk-relevant roles and states. Preserve ordinary form fallback; use keyboard and applicable accessibility checks. Fix findings and repeat affected journeys.
3. Record revision, viewport, role, actions, outcomes, evidence, and unresolved findings as required by [browser review](../../../docs/ux-review.md). If rendered review is unavailable, report blocked; source inspection, screenshots alone, and automated tests do not establish usability.
