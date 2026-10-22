package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

// renderArtifactHTML streams uploaded markup as inert base64. When Connection
// Allowlists are enabled, native WebRTC blocking is verified before parsing it;
// unsupported browsers fail closed.
func renderArtifactHTML(w io.Writer, source io.Reader, token string, connectionAllowlists bool) error {
	if len(token) > 128 {
		token = ""
	}
	encodedToken, _ := json.Marshal(token)
	if _, err := fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>Artifact preview</title></head><body><p id="preview-status">Checking preview restrictions…</p><script>
document.addEventListener("DOMContentLoaded", () => {
  const token = %s;
  const report = status => parent.postMessage({kind: "kamacu-artifact-preview", token, status}, "*");
`, encodedToken); err != nil {
		return err
	}
	if connectionAllowlists {
		if _, err := io.WriteString(w, `  const policyError = error => error.name === "NotAllowedError" && error.message.includes("Connection-Allowlist");
  let blocked = false;
  try {
    // An unsupported engine must reject this deliberately invalid ICE URL.
    // Policy-enforcing engines either reject construction explicitly because
    // of Connection-Allowlist or discard ICE servers before URL validation.
    // No valid network endpoint is supplied and ICE gathering is never started.
    const peer = new RTCPeerConnection({iceServers: [{urls: "kamacu-policy-probe:invalid"}], iceCandidatePoolSize: 0});
    blocked = peer.getConfiguration().iceServers.length === 0;
    peer.close();
  } catch (error) {
    blocked = policyError(error);
    // Earlier implementations validate configuration before checking policy.
    // Use a valid inert configuration only to obtain their explicit native
    // policy error; successful construction here is NOT evidence of blocking.
    if (!blocked && error.name === "SyntaxError") {
      try {
        const peer = new RTCPeerConnection({iceServers: [{urls: "stun:127.0.0.1:9"}], iceCandidatePoolSize: 0});
        peer.close();
      } catch (policyFailure) {
        blocked = policyError(policyFailure);
      }
    }
  }
  if (!blocked) {
    document.getElementById("preview-status").textContent = "This browser cannot enforce the preview's network restrictions. Use a browser with Connection Allowlists support enabled.";
    report("unsupported");
    return;
  }
`); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, `  const content = new TextDecoder().decode(Uint8Array.from(atob("`); err != nil {
		return err
	}
	// Base64 is streamed and cannot terminate this trusted script, even if the
	// uploaded HTML contains </script>, malformed encodings, or HTML parser tricks.
	encoder := base64.NewEncoder(base64.StdEncoding, w)
	if _, err := io.Copy(encoder, source); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	_, err := io.WriteString(w, `"), character => character.charCodeAt(0)));
  report("ready");
  // Wait for the wrapper parser to finish: document.open() is ignored when
  // called by a parser-executed inline script, leaving the status in the page.
  // document.open preserves the response's policy container; the new content
  // remains subject to the CSP sandbox and any enabled connection allowlist.
  document.open();
  document.write(content);
  document.close();
}, {once: true});
</script></body></html>`)
	return err
}
