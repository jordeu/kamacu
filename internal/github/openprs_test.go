package github

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListOpenPRs(t *testing.T) {
	for _, tc := range []struct {
		name, output, state string
		count               int
	}{
		{"pages", `[[{"number":7,"title":"Draft PR","draft":true,"user":{"login":"alice"}}],[{"number":8,"title":"Another PR","user":{"login":"bob"}}]]`, "ok", 2},
		{"empty", `[[]]`, "ok", 0},
		{"invalid", `invalid`, "error", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\n[ \"$1\" = api ] && [ \"$2\" = --paginate ] && [ \"$3\" = --slurp ] && [ \"$4\" = 'repos/owner/name/pulls?state=open&sort=updated&direction=desc&per_page=100' ] || exit 1\nprintf '%s' '" + tc.output + "'\n"
			if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			prs, state := ListOpenPRs(context.Background(), "owner/name", dir)
			if state != tc.state || len(prs) != tc.count {
				t.Fatalf("got %v %s", prs, state)
			}
			if tc.count > 0 && (prs[0].Author != "alice" || prs[1].Number != 8) {
				t.Fatalf("wrong decoded pages: %+v", prs)
			}
		})
	}
}

// A disconnected client or an expired deadline must terminate the gh process.
func TestListOpenPRsCancellation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nwhile :; do :; done\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan string, 1)
	go func() {
		_, state := ListOpenPRs(ctx, "owner/name", dir)
		done <- state
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case state := <-done:
		if state != "error" {
			t.Fatalf("state = %q, want error", state)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gh process did not stop after cancellation")
	}
}
