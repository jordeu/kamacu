-- +goose Up
CREATE TABLE hidden_pr_reviews (
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    repository TEXT NOT NULL COLLATE NOCASE,
    pr_number INTEGER NOT NULL CHECK (pr_number > 0),
    PRIMARY KEY (project_id, repository, pr_number)
);

-- +goose Down
DROP TABLE hidden_pr_reviews;
