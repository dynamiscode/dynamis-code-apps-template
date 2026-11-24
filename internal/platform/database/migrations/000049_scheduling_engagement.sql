ALTER TABLE scheduling_bookings ADD COLUMN engagement_id BIGINT REFERENCES engagements(id) ON DELETE SET NULL;
CREATE INDEX scheduling_bookings_engagement_idx ON scheduling_bookings (workspace_id, engagement_id);
