# Booking move browser review

Reviewed the #594 working tree with a disposable SQLite workspace and synthetic
booking. An owner moved a booking after an out-of-hours attempt; the error was
focused, retained the attempted time, and kept the move form open. Correcting
the time saved the move. At 390 × 844, history showed the old and new times,
actor, and timestamp without clipping. A viewer could expand history and saw
no move or outcome controls. Review and implementation were not independent.
