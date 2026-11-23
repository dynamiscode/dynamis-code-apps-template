# Evaluate a browser journey

Review against task success, not a subjective numerical score. Record concrete observations and affected states; distinguish actual evidence from assumptions.

## Three perspectives

1. **Journey:** Can the user locate the work, identify the correct record, see relevant context, complete the goal, and continue or return? Check first use, repeat work, and multiple similar records.
2. **Interaction and visual hierarchy:** Are the primary task, current state, and next action apparent? Check component fit, action placement, grouping, density, copy, visual alignment, and whether disclosure hides required information.
3. **Accessibility and implementation:** Can the same task be completed by keyboard and without JavaScript, with meaningful focus and labels, across relevant roles and sizes? Run applicable accessibility checks; check authorization and error recovery.

## Evidence and disposition

- For every changed journey record entry, role, sample data, viewport, actions, result, and evidence. Inspect desktop and narrow screens and risk-relevant empty, populated, error, success, and permission states.
- For each finding record location, observed action and outcome, user impact, severity, proposed remedy, and evidence. Separate verified defects from hypotheses and unsupported claims. Fix in-scope findings and re-run the affected path. State remaining issues and owners or blockers; do not mark a failed or unobserved journey as passed.
- Check avoidable cognitive work: repeated entry, recall between steps, unexplained choices, and competing actions. Preserve necessary safety and context; fewer clicks alone is not evidence of better usability.
- Use screenshots plus behavior evidence, not screenshots alone. Use disposable data and avoid credentials or personal information.
- A real-browser review is required for meaningful interaction or visual work. If unavailable, report blocked. Automated accessibility tests and heuristic review cannot establish compliance or replace testing with representative users.
