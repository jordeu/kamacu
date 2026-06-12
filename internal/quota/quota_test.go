package quota

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeToken is generated, never pasted — greppable-safe (108 chars, the
// observed sk-ant-oat01 length class). Real tokens must never enter fixtures.
func fakeToken() string {
	return "sk-ant-oat01-" + strings.Repeat("x", 95)
}

// writeCreds writes a credentials fixture into a temp dir and returns its path.
func writeCreds(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write creds fixture: %v", err)
	}
	return path
}

// liveUsageJSON mirrors the response captured live 2026-06-12: populated
// five_hour/seven_day, whole-null seven_day_opus, sonnet with null resets_at,
// plus the deferred extra_usage block (QUOTA-FUT-01) whose shape lacks
// utilization.
const liveUsageJSON = `{
  "five_hour":        { "utilization": 15, "resets_at": "2026-06-12T07:49:59Z" },
  "seven_day":        { "utilization": 73, "resets_at": "2026-06-13T20:59:59Z" },
  "seven_day_opus":   null,
  "seven_day_sonnet": { "utilization": 0, "resets_at": null },
  "extra_usage":      { "is_enabled": false }
}`

func TestReadCredentialsLiveShape(t *testing.T) {
	tok := fakeToken()
	// Exact live file shape: claudeAiOauth alongside unrelated mcpOAuth
	// secrets that must be ignored (decode only the two needed fields).
	path := writeCreds(t, `{
	  "claudeAiOauth": {
	    "accessToken": "`+tok+`",
	    "refreshToken": "fake-refresh-never-used",
	    "expiresAt": 1781238586291,
	    "scopes": ["user:inference"],
	    "subscriptionType": "team",
	    "rateLimitTier": "default"
	  },
	  "mcpOAuth": { "someServer": { "accessToken": "third-party-secret" } }
	}`)

	gotTok, gotExp, err := readCredentials(path)
	if err != nil {
		t.Fatalf("readCredentials: %v", err)
	}
	if gotTok != tok {
		t.Errorf("token = %q, want the fixture token", gotTok)
	}
	// expiresAt is epoch MILLISECONDS — must round-trip via time.UnixMilli.
	if want := time.UnixMilli(1781238586291); !gotExp.Equal(want) {
		t.Errorf("expiresAt = %v, want %v (UnixMilli of 1781238586291)", gotExp, want)
	}
}

func TestReadCredentialsNoCredentials(t *testing.T) {
	t.Run("file absent", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".credentials.json") // never written
		tok, exp, err := readCredentials(path)
		if !errors.Is(err, errNoCredentials) {
			t.Fatalf("err = %v, want errNoCredentials", err)
		}
		if tok != "" || !exp.IsZero() {
			t.Errorf("got (%q, %v), want zero values", tok, exp)
		}
	})

	t.Run("claudeAiOauth key absent", func(t *testing.T) {
		path := writeCreds(t, `{"mcpOAuth":{}}`)
		if _, _, err := readCredentials(path); !errors.Is(err, errNoCredentials) {
			t.Fatalf("err = %v, want errNoCredentials", err)
		}
	})

	t.Run("empty accessToken", func(t *testing.T) {
		path := writeCreds(t, `{"claudeAiOauth":{"accessToken":"","expiresAt":1781238586291}}`)
		if _, _, err := readCredentials(path); !errors.Is(err, errNoCredentials) {
			t.Fatalf("err = %v, want errNoCredentials", err)
		}
	})
}

func TestNormalizeWindowsLive(t *testing.T) {
	got, err := normalizeWindows([]byte(liveUsageJSON))
	if err != nil {
		t.Fatalf("normalizeWindows: %v", err)
	}

	// Exactly three windows, in the fixed preference order (D-75):
	// seven_day_opus (whole-null) skipped, extra_usage skipped.
	want := []struct {
		key   string
		label string
		util  float64
	}{
		{"five_hour", "5h", 15},
		{"seven_day", "7d", 73},
		{"seven_day_sonnet", "Sonnet", 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d windows %+v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].Key != w.key || got[i].Label != w.label || got[i].Utilization != w.util {
			t.Errorf("windows[%d] = {%q %q %v}, want {%q %q %v}",
				i, got[i].Key, got[i].Label, got[i].Utilization, w.key, w.label, w.util)
		}
	}

	// resets_at passthrough: populated for five_hour, nil carried for sonnet.
	if got[0].ResetsAt == nil || *got[0].ResetsAt != "2026-06-12T07:49:59Z" {
		t.Errorf("five_hour resetsAt = %v, want 2026-06-12T07:49:59Z", got[0].ResetsAt)
	}
	if got[2].ResetsAt != nil {
		t.Errorf("seven_day_sonnet resetsAt = %q, want nil", *got[2].ResetsAt)
	}
}

func TestNormalizeWindowsUnknownKeyPassthrough(t *testing.T) {
	// Unknown future keys come AFTER the known four, label = raw key (D-74).
	// extra_usage is skipped explicitly even if it grows a utilization field.
	body := `{
	  "weekly_haiku":     { "utilization": 12, "resets_at": null },
	  "five_hour":        { "utilization": 15, "resets_at": "2026-06-12T07:49:59Z" },
	  "seven_day":        { "utilization": 73, "resets_at": "2026-06-13T20:59:59Z" },
	  "seven_day_opus":   null,
	  "seven_day_sonnet": { "utilization": 0, "resets_at": null },
	  "extra_usage":      { "utilization": 50, "resets_at": null }
	}`
	got, err := normalizeWindows([]byte(body))
	if err != nil {
		t.Fatalf("normalizeWindows: %v", err)
	}
	wantKeys := []string{"five_hour", "seven_day", "seven_day_sonnet", "weekly_haiku"}
	if len(got) != len(wantKeys) {
		t.Fatalf("got %d windows %+v, want keys %v", len(got), got, wantKeys)
	}
	for i, k := range wantKeys {
		if got[i].Key != k {
			t.Errorf("windows[%d].Key = %q, want %q", i, got[i].Key, k)
		}
	}
	last := got[len(got)-1]
	if last.Label != "weekly_haiku" {
		t.Errorf("unknown-key label = %q, want raw key %q", last.Label, "weekly_haiku")
	}
	if last.Utilization != 12 {
		t.Errorf("weekly_haiku utilization = %v, want 12", last.Utilization)
	}
}

func TestNormalizeWindowsMalformed(t *testing.T) {
	if _, err := normalizeWindows([]byte("not json")); err == nil {
		t.Fatal("normalizeWindows(malformed) = nil error, want error")
	}
}
