package api

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestArtifactRendererKeepsUploadedMarkupInert(t *testing.T) {
	for _, content := range []string{
		`</script><script>fetch('https://example.com/escape')</script>`,
		"<!doctype html><h1>Résumé 日本語</h1><script>document.write('</script>')</script>",
		"\xff\x00<!-- parser edge --><svg onload='new RTCPeerConnection()'>",
	} {
		var output bytes.Buffer
		if err := renderArtifactHTML(&output, strings.NewReader(content), `</script><script>alert(1)</script>`, true); err != nil {
			t.Fatal(err)
		}
		document := output.String()
		if strings.Count(document, "<script>") != 1 || strings.Count(document, "</script>") != 1 {
			t.Fatal("uploaded content or token escaped trusted script")
		}
		_, encoded, ok := strings.Cut(document, `atob("`)
		if !ok {
			t.Fatal("missing inert payload")
		}
		encoded, _, ok = strings.Cut(encoded, `")`)
		if !ok {
			t.Fatal("unterminated inert payload")
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || !bytes.Equal(decoded, []byte(content)) {
			t.Fatal("payload changed", err)
		}
	}
}

func TestArtifactRendererConnectionAllowlistsOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		var output bytes.Buffer
		if err := renderArtifactHTML(&output, strings.NewReader("<h1>Preview</h1>"), "token", enabled); err != nil {
			t.Fatal(err)
		}
		document := output.String()
		if strings.Contains(document, "new RTCPeerConnection") != enabled ||
			strings.Contains(document, `report("unsupported")`) != enabled {
			t.Fatalf("browser capability gate does not match opt-in %v", enabled)
		}
		if !strings.Contains(document, `report("ready")`) || !strings.Contains(document, "document.write(content)") {
			t.Fatal("preview must render and report readiness in either mode")
		}
	}
}

func TestArtifactRendererBrowser(t *testing.T) {
	if os.Getenv("KAMACU_BROWSER_TEST") != "1" {
		t.Skip("set KAMACU_BROWSER_TEST=1 to run browser acceptance")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; script-src 'unsafe-inline'")
		if err := renderArtifactHTML(w, strings.NewReader(`<h1>Preview</h1><script>document.body.dataset.ran = 'yes'</script>`), "token", false); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	cmd := exec.Command("node", "-e", `
 const {chromium} = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
 const assert = require('node:assert/strict');
 (async () => {
  const browser = await chromium.launch({headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox']});
  try {
   const page = await browser.newPage();
   await page.goto(process.argv[1]);
   await page.locator('body[data-ran="yes"]').waitFor();
   assert.equal(await page.locator('h1').textContent(), 'Preview');
   assert.equal(await page.locator('#preview-status').count(), 0, 'wrapper status remained above the artifact');
  } finally { await browser.close(); }
 })().catch(error => { console.error(error); process.exitCode = 1; });
 `, server.URL)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser acceptance: %v\n%s", err, output)
	}
}
