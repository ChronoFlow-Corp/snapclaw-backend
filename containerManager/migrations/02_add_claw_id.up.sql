ALTER TABLE containers
    ADD COLUMN IF NOT EXISTS claw_id varchar(64);

UPDATE containers
SET claw_id = ''
WHERE claw_id IS NULL;

ALTER TABLE containers
    ALTER COLUMN claw_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS containers_user_claw_uidx
    ON containers (user_id, claw_id);
