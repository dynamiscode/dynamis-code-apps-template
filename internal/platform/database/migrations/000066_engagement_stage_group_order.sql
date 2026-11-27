UPDATE engagement_stages SET position = -position;

WITH ordered AS MATERIALIZED (
    SELECT id, ROW_NUMBER() OVER (
        PARTITION BY workspace_id
        ORDER BY CASE kind WHEN 'open' THEN 0 WHEN 'won' THEN 1 ELSE 2 END, -position
    ) AS new_position
    FROM engagement_stages
)
UPDATE engagement_stages SET position = (SELECT new_position FROM ordered WHERE ordered.id = engagement_stages.id);
