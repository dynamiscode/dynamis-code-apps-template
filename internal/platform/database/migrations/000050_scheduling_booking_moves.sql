CREATE TABLE scheduling_booking_moves (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    booking_id BIGINT NOT NULL REFERENCES scheduling_bookings(id) ON DELETE CASCADE,
    actor_id BIGINT NOT NULL,
    old_start TEXT NOT NULL,
    old_end TEXT NOT NULL,
    new_start TEXT NOT NULL,
    new_end TEXT NOT NULL,
    moved_at TEXT NOT NULL
);
