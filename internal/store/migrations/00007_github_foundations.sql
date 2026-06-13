-- +goose Up
-- v1.3 GitHub foundations (D-12). One ALTER ... ADD COLUMN per statement
-- (modernc/SQLite, as 00006). projects.description defaults '' (NOT NULL);
-- projects.github_repo is nullable (NULL = not linked, D-10). The three
-- tasks columns land NOW so Phase 12 needs no further migration: source
-- discriminates manual vs PR-backed tasks (board excludes PRs via
-- WHERE source='manual' in Phase 12); pr_number/pr_base_ref are NULL for
-- manual tasks.
ALTER TABLE projects ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN github_repo TEXT;
ALTER TABLE tasks ADD COLUMN source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','github_pr'));
ALTER TABLE tasks ADD COLUMN pr_number INTEGER;
ALTER TABLE tasks ADD COLUMN pr_base_ref TEXT;

-- +goose Down
-- modernc.org/sqlite tracks SQLite 3.53, which supports DROP COLUMN directly
-- (no table rebuild). One DROP per statement, reverse order of the adds.
ALTER TABLE tasks DROP COLUMN pr_base_ref;
ALTER TABLE tasks DROP COLUMN pr_number;
ALTER TABLE tasks DROP COLUMN source;
ALTER TABLE projects DROP COLUMN github_repo;
ALTER TABLE projects DROP COLUMN description;
