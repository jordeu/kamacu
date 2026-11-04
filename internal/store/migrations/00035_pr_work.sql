-- +goose Up
CREATE TABLE task_pr_links (
 task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 repository TEXT NOT NULL,
 number INTEGER NOT NULL CHECK(number > 0),
 role TEXT NOT NULL CHECK(role IN ('implementation','review','incidental')),
 source TEXT NOT NULL,
 evidence TEXT NOT NULL,
 uncertainty TEXT NOT NULL,
 observed_at TEXT NOT NULL,
 corrected INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(task_id,repository,number)
);
CREATE INDEX task_pr_reverse ON task_pr_links(repository,number);
INSERT INTO task_pr_links(task_id,repository,number,role,source,evidence,uncertainty,observed_at)
 SELECT t.id,lower(p.github_repo),t.pr_number,'review','review_task','Existing review workspace','',strftime('%Y-%m-%dT%H:%M:%f',t.updated_at)||'000000Z'
 FROM tasks t JOIN projects p ON p.id=t.project_id
 WHERE t.source='github_pr' AND t.pr_number>0 AND p.github_repo IS NOT NULL;
CREATE TABLE session_work (
 session_id TEXT PRIMARY KEY,
 task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
 state TEXT NOT NULL,
 summary TEXT NOT NULL,
 evidence TEXT NOT NULL,
 uncertainty TEXT NOT NULL,
 observed_at TEXT NOT NULL
);
CREATE TABLE session_pr_links (
 session_id TEXT NOT NULL,
 repository TEXT NOT NULL,
 number INTEGER NOT NULL,
 role TEXT NOT NULL,
 source TEXT NOT NULL,
 evidence TEXT NOT NULL,
 uncertainty TEXT NOT NULL,
 observed_at TEXT NOT NULL,
 PRIMARY KEY(session_id,repository,number)
);
CREATE INDEX session_pr_reverse ON session_pr_links(repository,number);
-- +goose Down
DROP TABLE session_pr_links;
DROP TABLE session_work;
DROP TABLE task_pr_links;
