ALTER TABLE containers
    ADD COLUMN IF NOT EXISTS has_started_once boolean NOT NULL DEFAULT false;
