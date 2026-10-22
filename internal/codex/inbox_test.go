package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFeedbackEnvironmentArgs(t *testing.T) {
	data := `[
 {"name":"kamacu","enabled":true,"transport":{"type":"stdio","command":"wrapper","env_vars":["EXISTING","KAMACU_TASK_ID"]}},
 {"name":"renamed","enabled":true,"transport":{"type":"stdio","command":"/usr/bin/kamacu","args":["mcp","serve"]}},
 {"name":"other","enabled":true,"transport":{"type":"stdio","command":"other"}},
 {"name":"disabled","enabled":false,"transport":{"type":"stdio","command":"kamacu","args":["mcp","serve"]}},
 {"name":"remote","enabled":true,"transport":{"type":"http","command":"kamacu","args":["mcp","serve"]}}
 ]`
	got, err := feedbackEnvironmentArgs([]byte(data))
	want := []string{"-c", `mcp_servers."kamacu".env_vars=["EXISTING","KAMACU_TASK_ID","KAMACU_SESSION_ID","KAMACU_HOOK_TOKEN","KAMACU_HOOK_BASE"]`, "-c", `mcp_servers."renamed".env_vars=["KAMACU_TASK_ID","KAMACU_SESSION_ID","KAMACU_HOOK_TOKEN","KAMACU_HOOK_BASE"]`}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, %v", got, err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	script := "#!/bin/sh\n[ \"$*\" = '-p kamacu mcp list --json' ] || exit 1\ncat <<'DATA'\n" + data + "\nDATA\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	got, err = FeedbackEnvironmentArgs(bin, dir)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("discovery got %v, %v", got, err)
	}
	if _, err := feedbackEnvironmentArgs([]byte("bad secret output")); err == nil {
		t.Fatal("accepted malformed response")
	}
	if got, err := feedbackEnvironmentArgs([]byte("[]")); err != nil || len(got) != 0 {
		t.Fatalf("empty registrations: %v %v", got, err)
	}
}
