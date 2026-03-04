CREATE TABLE IF NOT EXISTS containers
(
    id           uuid PRIMARY KEY,
    user_id      varchar(64)  NOT NULL,
    container_id varchar(128) NOT NULL UNIQUE,
    status       varchar(64)  NOT NULL DEFAULT 'stop',
    port         int          NOT NULL UNIQUE,
    created_at   timestamptz           DEFAULT current_timestamp,
    updated_at   timestamptz           DEFAULT current_timestamp
)