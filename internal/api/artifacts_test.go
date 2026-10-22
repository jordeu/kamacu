package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/artifact"
	"kamacu/internal/store"
)

func artifactServer(t *testing.T) (*httptest.Server, *artifact.Service) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO projects(id,name,repo_path) VALUES(1,'Artifacts','/tmp/example')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Preview artifacts',0)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := artifact.New(db, filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	ArtifactRoutes(mux, svc, "test-token")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		root := "../../web/dist"
		if _, err := os.Stat(filepath.Join(root, r.URL.Path)); err == nil && r.URL.Path != "/" {
			http.FileServer(http.Dir(root)).ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
	server := httptest.NewServer(BrowserOriginGuard(mux, nil))
	t.Cleanup(server.Close)
	return server, svc
}

func TestArtifactHTTPUploadAndIsolationHeaders(t *testing.T) {
	for _, setting := range []string{"", "0", "false", "invalid", "1", "true"} {
		t.Run("allowlists="+setting, func(t *testing.T) {
			t.Setenv("KAMACU_ARTIFACT_CONNECTION_ALLOWLISTS", setting)
			testArtifactHTTPUploadAndIsolationHeaders(t, setting == "1" || setting == "true")
		})
	}
}

func testArtifactHTTPUploadAndIsolationHeaders(t *testing.T, enabled bool) {
	srv, svc := artifactServer(t)
	request := func(method, path, token string, body []byte) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if method == http.MethodPut {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set("X-Kamacu-Token", token)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	payload := []byte(`<h1>Preview</h1><script>document.body.dataset.ran='yes'</script>`)
	input := []byte(fmt.Sprintf(`{"title":"Demo","filename":"demo.html","size_bytes":%d}`, len(payload)))
	if resp := request("POST", "/api/tasks/1/artifact-uploads", "", input); resp.StatusCode != 401 {
		t.Fatal(resp.Status)
	}
	resp := request("POST", "/api/tasks/1/artifact-uploads", "test-token", input)
	if resp.StatusCode != 201 {
		t.Fatal(resp.Status)
	}
	var u struct {
		ID      string            `json:"upload_id"`
		URL     string            `json:"upload_url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		t.Fatal(err)
	}
	if resp = request("PUT", u.URL, "Bearer wrong", payload); resp.StatusCode != 401 {
		t.Fatal(resp.Status)
	}
	if resp = request("PUT", u.URL, u.Headers["Authorization"], payload); resp.StatusCode != 204 {
		t.Fatal(resp.Status)
	}
	resp = request("POST", "/api/tasks/1/artifact-uploads/"+u.ID+"/finalize", "test-token", nil)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatal(resp.Status, string(body))
	}
	var a artifact.Artifact
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		t.Fatal(err)
	}
	file := fmt.Sprintf("/api/artifact-content/%s/%s/index.html", a.ID, a.Revision)
	resp = request("GET", file, "", nil)
	csp := resp.Header.Get("Content-Security-Policy")
	if resp.StatusCode != 200 || !strings.Contains(csp, "sandbox allow-scripts;") || strings.Contains(csp, "allow-same-origin") || !strings.Contains(csp, "default-src 'none'") {
		t.Fatal(resp.Status, csp)
	}
	if strings.Contains(csp, "connect-src 'self'") || !strings.Contains(csp, srv.URL+fmt.Sprintf("/api/artifact-content/%s/%s/", a.ID, a.Revision)) {
		t.Fatal("asset scope too broad", csp)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal(resp.Header)
	}
	wantPolicy := ""
	if enabled {
		wantPolicy = fmt.Sprintf("(%q);webrtc=block;redirects=block", srv.URL+fmt.Sprintf("/api/artifact-content/%s/%s/*", a.ID, a.Revision))
	}
	if resp.Header.Get("Connection-Allowlist") != wantPolicy {
		t.Fatal("missing scoped native network policy", resp.Header)
	}
	content, err := io.ReadAll(resp.Body)
	if err != nil || bytes.Contains(content, payload) || bytes.Contains(content, []byte(`report("unsupported")`)) != enabled {
		t.Fatal("HTML must remain inert until browser policy verification", err)
	}
	if resp = request("GET", strings.Replace(file, a.Revision, "missing", 1), "", nil); resp.StatusCode != 404 {
		t.Fatal(resp.Status)
	}
	if resp = request("PATCH", "/api/tasks/1/artifacts/"+a.ID, "", []byte(`{"title":"Renamed"}`)); resp.StatusCode != 200 {
		t.Fatal(resp.Status)
	}
	if resp = request("PATCH", "/api/tasks/1/artifacts/"+a.ID, "", []byte(`{"title":""}`)); resp.StatusCode != 400 {
		t.Fatal(resp.Status)
	}
	if resp = request("DELETE", "/api/tasks/1/artifacts/"+a.ID, "", nil); resp.StatusCode != 204 {
		t.Fatal(resp.Status)
	}
	if resp = request("GET", file, "", nil); resp.StatusCode != 404 {
		t.Fatal(resp.Status)
	}
	if list, err := svc.List(context.Background(), 1); err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
}

func TestArtifactActiveAssetsCannotExecuteOnDirectNavigation(t *testing.T) {
	srv, svc := artifactServer(t)
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for _, file := range []struct{ name, content string }{
		{"index.html", "<h1>Preview</h1>"},
		{"active.svg", `<svg xmlns="http://www.w3.org/2000/svg"><script>new RTCPeerConnection()</script></svg>`},
		{"active.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><script>new RTCPeerConnection()</script></html>`},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: file.name, Mode: 0600, Size: int64(len(file.content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, file.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u, err := svc.Begin(ctx, 1, "", "Active assets", "bundle.tar.gz", int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Receive(ctx, u.ID, u.Token, bytes.NewReader(archive.Bytes())); err != nil {
		t.Fatal(err)
	}
	a, err := svc.Finalize(ctx, 1, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"active.svg", "active.xhtml"} {
		response, err := srv.Client().Get(fmt.Sprintf("%s/api/artifact-content/%s/%s/%s", srv.URL, a.ID, a.Revision, file))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		policy := response.Header.Get("Content-Security-Policy")
		if response.StatusCode != 200 || !strings.HasPrefix(policy, "sandbox;") || strings.Contains(policy, "allow-scripts") {
			t.Fatal("active asset can execute uploaded code", file, policy)
		}
		if response.Header.Get("Content-Disposition") != "attachment" {
			t.Fatal("active asset can render unverified navigation links", file)
		}
	}
}

// Opt-in real-browser acceptance test. Uses the built web app and a disposable
// HTTP service/database. No live Kamacu instance or shared upload path is used.
func TestArtifactsBrowser(t *testing.T) {
	t.Setenv("KAMACU_ARTIFACT_CONNECTION_ALLOWLISTS", "1")
	if os.Getenv("KAMACU_BROWSER_TEST") != "1" {
		t.Skip("set KAMACU_BROWSER_TEST=1 and KAMACU_PLAYWRIGHT_PACKAGE to run browser acceptance")
	}
	srv, _ := artifactServer(t)
	cmd := exec.Command("node", "../../scripts/artifacts-browser-check.cjs", srv.URL)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser acceptance: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
