-- +goose Up
-- Keep the initial review prompt obligation across notification runs and crashes.
CREATE TABLE notification_review_prompts (
    task_id INTEGER PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    prompt TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'sending', 'sent'))
);
-- +goose Down
DROP TABLE notification_review_prompts;
