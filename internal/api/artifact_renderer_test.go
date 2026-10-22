package api

import (
	"bytes"
	"encoding/base64"
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
		if err := renderArtifactHTML(&output, strings.NewReader(content), `</script><script>alert(1)</script>`); err != nil {
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
