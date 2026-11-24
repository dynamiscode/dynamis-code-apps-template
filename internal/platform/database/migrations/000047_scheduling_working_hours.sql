CREATE TABLE scheduling_working_hours (
    resource_id BIGINT NOT NULL REFERENCES scheduling_resources(id) ON DELETE CASCADE,
    weekday INTEGER NOT NULL,
    start_time TEXT NOT NULL,
    end_time TEXT NOT NULL
);

CREATE INDEX scheduling_working_hours_resource_idx ON scheduling_working_hours (resource_id);
