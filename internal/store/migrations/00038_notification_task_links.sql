-- +goose Up
CREATE TABLE notification_task_links (
 question_id TEXT NOT NULL REFERENCES inbox_questions(id) ON DELETE CASCADE,
 task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 role TEXT NOT NULL CHECK(role IN ('implementation','review','recurring','investigation')),
 source TEXT NOT NULL,
 evidence TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(question_id,task_id)
);
-- Retain navigable task relationships for existing development inbox history.
INSERT OR IGNORE INTO notification_task_links(question_id,task_id,role,source,evidence)
 SELECT p.question_id,t.id,CASE WHEN t.source='github_pr' THEN 'review' WHEN t.recurrent=1 THEN 'recurring' ELSE 'implementation' END,'saved_plan','Previously selected task'
 FROM notification_card_plans p JOIN tasks t ON t.id=json_extract(p.plan,'$.task_id');
INSERT OR IGNORE INTO notification_task_links(question_id,task_id,role,source,evidence)
 SELECT p.question_id,t.id,l.role,l.source,l.evidence FROM notification_card_plans p,json_each(p.plan,'$.targets') target
 JOIN projects project ON project.id=json_extract(target.value,'$.project_id')
 JOIN task_pr_links l ON lower(l.repository)=lower(project.github_repo) AND l.number=json_extract(target.value,'$.pr_number')
 JOIN tasks t ON t.id=l.task_id
 WHERE l.role IN ('implementation','review');
INSERT OR IGNORE INTO notification_task_links(question_id,task_id,role,source,evidence)
 SELECT p.question_id,t.id,COALESCE(explicit.role,l.role),l.source,l.evidence
 FROM notification_card_plans p,json_each(p.plan,'$.targets') target
 JOIN projects project ON project.id=json_extract(target.value,'$.project_id')
 JOIN session_pr_links l ON lower(l.repository)=lower(project.github_repo) AND l.number=json_extract(target.value,'$.pr_number')
 LEFT JOIN session_work w ON w.session_id=l.session_id
 JOIN tasks t ON t.id=COALESCE(l.task_id,w.task_id)
 LEFT JOIN task_pr_links explicit ON explicit.task_id=t.id AND lower(explicit.repository)=lower(l.repository) AND explicit.number=l.number
 WHERE COALESCE(explicit.role,l.role) IN ('implementation','review');
INSERT OR IGNORE INTO notification_task_links(question_id,task_id,role,source,evidence)
 SELECT p.question_id,t.id,'review','review_identity','Existing review workspace for the project and PR'
 FROM notification_card_plans p,json_each(p.plan,'$.targets') target
 JOIN tasks t ON t.project_id=json_extract(target.value,'$.project_id') AND t.pr_number=json_extract(target.value,'$.pr_number') AND t.source='github_pr'
 WHERE json_extract(p.plan,'$.workflow')='route_review' OR json_extract(p.plan,'$.work_kind')='review';
-- Pending cards must be regenerated with ownership-preserving action plans.
-- Their task links remain available in archived history.
UPDATE notification_events SET raw_status='pending',held=0,raw_revision=raw_revision+1,state='queued'
 WHERE id IN (SELECT e.event_id FROM notification_question_events e JOIN inbox_questions q ON q.id=e.question_id WHERE q.origin='notification' AND (q.status='pending' OR q.delivery_status='queued')
 UNION SELECT notification_id FROM inbox_questions WHERE origin='notification' AND (status='pending' OR delivery_status='queued') AND notification_id IS NOT NULL);
UPDATE inbox_questions SET status='unanswered',reason='notification_task_routing_changed',revision=revision+1 WHERE origin='notification' AND status='pending';
UPDATE notification_batches SET status='failed',error='Notification task routing changed; process pending inputs again.' WHERE status IN ('queued','running');
UPDATE inbox_questions SET delivery_status='failed',execution_result='Notification task routing changed before execution; process notifications again.' WHERE origin='notification' AND delivery_status='queued';
UPDATE notification_runs SET state='failed' WHERE state='queued';
-- +goose Down
DROP TABLE notification_task_links;
