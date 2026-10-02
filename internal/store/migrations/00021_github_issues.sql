-- +goose Up
ALTER TABLE tasks ADD COLUMN issue_repo TEXT COLLATE NOCASE;
ALTER TABLE tasks ADD COLUMN issue_number INTEGER;
ALTER TABLE tasks ADD COLUMN issue_url TEXT;
CREATE UNIQUE INDEX tasks_github_issue ON tasks(project_id, issue_repo, issue_number)
 WHERE issue_repo IS NOT NULL AND issue_number IS NOT NULL;

-- +goose Down
DROP INDEX tasks_github_issue;
ALTER TABLE tasks DROP COLUMN issue_url;
ALTER TABLE tasks DROP COLUMN issue_number;
ALTER TABLE tasks DROP COLUMN issue_repo;
