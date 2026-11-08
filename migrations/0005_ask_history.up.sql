-- Saved AI assistant conversations, one row per answered question. Owned by the assistant module.
-- username comes from the caller's token; there is deliberately no foreign key to users (module boundary).
-- parts is a snapshot of the rows the answer was based on, so a saved answer never changes after the fact.
CREATE TABLE IF NOT EXISTS ask_history (
    id              bigserial PRIMARY KEY,
    username        text        NOT NULL,
    question        text        NOT NULL,
    answer          text        NOT NULL,
    tools_used      jsonb       NOT NULL DEFAULT '[]',
    parts           jsonb       NOT NULL DEFAULT '[]',
    parts_truncated boolean     NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ask_history_user_id_idx ON ask_history (username, id DESC);
