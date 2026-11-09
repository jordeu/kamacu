package notifications

import "slices"

func actionObject(properties map[string]any) map[string]any {
	keys := []string{}
	for k := range properties {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": keys}
}
func actionArray(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
func actionSchema() map[string]any {
	str := map[string]any{"type": "string"}
	id := map[string]any{"type": "integer"}
	return actionObject(map[string]any{"busy_policy": map[string]any{"type": "string", "enum": []string{"", "wait_idle", "send_now"}}, "id": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{1,60}$"}, "kind": map[string]any{"type": "string", "enum": []string{"start_reviews", "run_recurring_task", "continue_task", "create_investigation", "send_to_session", "close_session", "complete_task", "update_task", "associate_pr", "ignore", "open_task"}}, "project_id": id, "task_id": id, "task_ref": str, "session_id": str, "pr_numbers": actionArray(id), "task_name": str, "instructions": str, "status": str, "title": str, "description": str, "link_role": str})
}
func agentOptionSchema() map[string]any {
	str := map[string]any{"type": "string"}
	return actionObject(map[string]any{"id": str, "label": str, "mode": map[string]any{"type": "string", "enum": []string{"action", "reply"}}, "actions": actionArray(actionSchema())})
}
