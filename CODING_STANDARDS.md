# Coding standards

This file lists judgement-call standards for review. Mechanical rules belong in tooling, not here.

## Behavior docs

Behavior docs describe only the paths the code covers. Do not write "every", "always", "all", or "each path" claims unless the code or a test enumerates each case. When a path is an exception (for example, a state that waits until the next request), name it in the same paragraph.
