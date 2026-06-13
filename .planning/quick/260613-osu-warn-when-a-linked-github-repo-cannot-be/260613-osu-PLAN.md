---
phase: quick-260613-osu
plan: 01
type: tdd
wave: 1
depends_on: []
files_modified:
  - internal/api/projects.go
  - internal/api/projects_test.go
  - web/src/api/types.ts
autonomous: true
requirements: [GHPRJ-03, D-11]

must_haves:
  truths:
    - "Linking a syntactically-valid but unverifiable GitHub repo still saves (200), and the response carries a verify_state advisory the dialog can surface"
    - "A description-only or name-only PATCH returns NO verify_state field (byte-for-byte identical to today's response)"
    - "Unlinking a repo (github_repo: \"\") returns NO verify_state field and github_repo null"
    - "A syntactically invalid github_repo still hard-blocks with 400 (unchanged)"
  artifacts:
    - path: "internal/api/projects.go"
      provides: "verify_state computed from ValidateRepo's verified bool + github.Available(), returned via a response-only wrapper struct"
      contains: "projectUpdateResponse"
    - path: "internal/api/projects_test.go"
      provides: "host-independent test asserting verify_state appears only when a non-empty repo was set and gh could not verify it"
      contains: "TestUpdateProjectVerifyState"
  key_links:
    - from: "internal/api/projects.go update handler"
      to: "github.ValidateRepo verified bool + github.Available()"
      via: "repoSet/verified locals carried past the SET-clause block, computed after scanProject"
      pattern: "verify_state"
    - from: "internal/api/projects.go projectUpdateResponse"
      to: "web/src/components/sidebar/ProjectSettingsDialog.tsx (already wired)"
      via: "snake_case verify_state field with omitempty"
      pattern: "VerifyState string `json:\"verify_state,omitempty\"`"
---

<objective>
Complete Phase 10's "soft-save-with-warning" contract (GHPRJ-03 / D-11). The PATCH
/api/projects/{id} handler already SAVES a syntactically-valid-but-unverifiable
GitHub repo (degrade-don't-break) but throws away the `verified` bool from
`github.ValidateRepo` and returns a plain 200. The frontend dialog
(ProjectSettingsDialog.tsx) is ALREADY wired to read `updated.verify_state` and
render a muted advisory ("no_gh" → gh-not-available copy, "unverifiable" →
couldn't-verify-{repo} copy), keeping the dialog open. Today that field is never
sent, so the advisory is dormant.

This plan makes the server emit `verify_state` so the dormant advisory activates.

Purpose: A user who links a repo `gh` can't confirm (gh missing, repo not found,
or no access) sees a non-blocking "saved, but couldn't verify" notice instead of
silent success — without ever blocking the save.

Output: A response-only `verify_state` field on PATCH; no DB column, no
scanProject change, no schema migration, no frontend behavior change.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/STATE.md
@CLAUDE.md

# The single file to change (handler + Project struct + scanProject + projectColumns):
@internal/api/projects.go
# The leaf package whose verified bool + Available() drive verify_state:
@internal/github/github.go
# The test harness + the existing PATCH test to mirror (RED→GREEN style):
@internal/api/projects_test.go
# The host-independent assertion pattern to copy (branch on github.Available()):
@internal/api/github_test.go
# The contract the server must satisfy — READ ONLY, DO NOT CHANGE (already wired):
@web/src/components/sidebar/ProjectSettingsDialog.tsx

<interfaces>
<!-- Contracts the executor needs. Use these directly — no exploration needed. -->

From internal/github/github.go:
```go
// ValidateRepo returns (canonical owner/name, verified bool, err).
//   - ParseRepoRef errors                 → ("", false, err)   [HARD block — already handled]
//   - gh absent (LookPath fails)          → (parsed, false, nil) [soft save → verify_state "no_gh"]
//   - gh present, `gh repo view` exit 0   → (nameWithOwner, true, nil) [verified → no verify_state]
//   - gh present, any nonzero / parse err → (parsed, false, nil) [soft save → verify_state "unverifiable"]
func ValidateRepo(ctx context.Context, ref string) (canonical string, verified bool, err error)

// Available reports whether `gh` resolves on PATH (call-time LookPath).
func Available() bool
```

From internal/api/projects.go (the file being edited):
```go
type Project struct {
    ID          int64   `json:"id"`
    Name        string  `json:"name"`
    RepoPath    string  `json:"repo_path"`
    Description string  `json:"description"`
    GithubRepo  *string `json:"github_repo"`
    CreatedAt   string  `json:"created_at"`
    UpdatedAt   string  `json:"updated_at"`
}
// In update(): line ~184 currently `canonical, _, err := github.ValidateRepo(...)`
// Final response: line ~209 currently `writeJSON(w, http.StatusOK, p)`
```

From internal/api/respond.go:
```go
func writeJSON(w http.ResponseWriter, status int, v any) // sets Content-Type, encodes v
```

From internal/api/projects_test.go (reuse, do not redefine):
```go
func newTestServer(t *testing.T) (*httptest.Server, *sql.DB, string)
func gitRepo(t *testing.T) string
func doJSON(t *testing.T, method, url string, body any) (int, map[string]any)
func createProject(t *testing.T, srv *httptest.Server, repoPath string) int64
```
</interfaces>
</context>

<important_caveat>
HOST STATE CORRECTION (verified at plan time, 2026-06-13): `gh` IS PRESENT on
this host (`command -v gh` resolves). The original task brief assumed gh was
renamed/absent and that "no_gh" would be the live deterministic path — that is
NO LONGER TRUE. Therefore the test MUST be host-independent (branch on
`github.Available()`, exactly like TestGithubStatus does), NOT hardcoded to
"no_gh". See Task 1's behavior block for the deterministic strategy that covers
BOTH the gh-present (`unverifiable`) and gh-absent (`no_gh`) hosts.

True existence verification needs gh present AND authenticated. A bogus repo
name returns nonzero from `gh repo view` whether gh is unauthenticated OR the
repo simply doesn't exist, so "unverifiable" is deterministic on a gh-present
host regardless of auth state.
</important_caveat>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: RED — failing test for verify_state on PATCH (host-independent)</name>
  <files>internal/api/projects_test.go</files>
  <behavior>
    Add a NEW test `TestUpdateProjectVerifyState` (do not modify the existing
    TestUpdateProjectPartial). Mirror the RED→GREEN style of the repo and the
    host-independent branching of TestGithubStatus. Reuse newTestServer/gitRepo/
    createProject/doJSON. Import "kangent/internal/github" (github_test.go in the
    same package already imports it, so the package compiles with it).

    Cases (all on one project; url = fmt.Sprintf("%s/api/projects/%d", srv.URL, id)):

    1. PATCH {"github_repo":"octocat/this-repo-does-not-exist-kangent-test"}
       — a SYNTACTICALLY valid ref that gh cannot confirm (does not exist).
       Assert status 200 and body["github_repo"] == "octocat/this-repo-does-not-exist-kangent-test".
       Then assert the advisory is correct FOR THIS HOST:
         vs, _ := body["verify_state"].(string)
         if github.Available() {
             want "unverifiable"   // gh present, `gh repo view <bogus>` exits nonzero
         } else {
             want "no_gh"          // gh absent on PATH
         }
       This single case exercises whichever advisory branch the host produces and
       is green on ANY host — it is the load-bearing assertion.

    2. PATCH {"description":"only desc"} (no github_repo key)
       — Assert status 200, body["description"] == "only desc", and
       _, ok := body["verify_state"]; ok == false (field ABSENT — omitempty).
       Proves a non-repo update is byte-for-byte identical to today.

    3. PATCH {"github_repo":""} (explicit unlink)
       — Assert status 200, body["github_repo"] == nil, and verify_state ABSENT.
       Proves unlink (the empty-string branch where ValidateRepo never runs)
       emits no advisory.

    Run `go test ./internal/api/ -run TestUpdateProjectVerifyState`: it MUST FAIL
    (compile error or the verify_state assertions) because the handler does not
    yet emit the field. This is RED.
  </behavior>
  <action>
    Append `TestUpdateProjectVerifyState` to internal/api/projects_test.go per the
    behavior block. Add `"kangent/internal/github"` to the import block (it is the
    same package `api`; github_test.go already imports it so there is no new module
    edge). Do NOT touch TestUpdateProjectPartial or any helper.

    Commit RED: `test(quick-260613-osu): add failing test for PATCH verify_state advisory`
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/api/ -run TestUpdateProjectVerifyState 2>&1 | grep -qE 'FAIL|build failed|cannot|undefined' && echo RED-OK</automated>
  </verify>
  <done>New test compiles-or-fails as RED: with the unchanged handler it does not pass (verify_state never present). Existing tests untouched.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — emit verify_state from the update handler via a response-only wrapper</name>
  <files>internal/api/projects.go</files>
  <behavior>
    Make TestUpdateProjectVerifyState (Task 1) PASS while leaving every other test
    green, with ZERO change to the DB schema, scanProject, projectColumns, the
    Project struct, or any error path.

    Behavior the handler must now have:
      - verify_state is non-empty ONLY when the request set a NON-EMPTY github_repo
        (the branch where ValidateRepo actually ran) AND verified == false.
          * "no_gh"        when !github.Available()
          * "unverifiable" otherwise (gh present but couldn't confirm)
      - verify_state is "" (→ omitted by omitempty) when: verified == true, the repo
        was unlinked via explicit "" (ValidateRepo never ran), or github_repo was
        absent from the request entirely.
  </behavior>
  <action>
    In internal/api/projects.go, edit ONLY the `update` handler and add ONE struct.

    1. Add a response-only wrapper near the Project struct (do NOT add DB tags, do
       NOT touch scanProject/projectColumns):
       ```go
       // projectUpdateResponse is the PATCH response: a Project plus a TRANSIENT,
       // non-persisted verify_state advisory (D-11). omitempty keeps the body
       // byte-for-byte identical to a bare Project whenever verify_state is "".
       type projectUpdateResponse struct {
           Project
           VerifyState string `json:"verify_state,omitempty"`
       }
       ```

    2. Carry two locals out of the `if req.GithubRepo != nil` block so verify_state
       can be computed AFTER scanProject. Before the block declare:
       ```go
       var repoSet, repoVerified bool
       ```
       In the non-empty branch, change line ~184 from `canonical, _, err := ...` to:
       ```go
       canonical, verified, err := github.ValidateRepo(r.Context(), *req.GithubRepo)
       ```
       and after the existing error check / before appending the SET clause, record:
       ```go
       repoSet = true
       repoVerified = verified
       ```
       (Leave the explicit-"" unlink branch alone: repoSet stays false there.)

    3. Replace the final `writeJSON(w, http.StatusOK, p)` (line ~209) with:
       ```go
       verifyState := ""
       if repoSet && !repoVerified {
           if !github.Available() {
               verifyState = "no_gh"
           } else {
               verifyState = "unverifiable"
           }
       }
       writeJSON(w, http.StatusOK, projectUpdateResponse{Project: p, VerifyState: verifyState})
       ```

    Do not change the 400/404/500 paths, the list/create/delete handlers, or any
    JSON tag on Project. `go vet`-clean: the embedded Project's fields still
    serialize at the top level, so existing clients see an unchanged body plus the
    optional verify_state.

    Commit GREEN: `feat(quick-260613-osu): surface verify_state advisory on soft-saved GitHub link (GHPRJ-03)`
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && go test ./internal/api/ && go build ./... && gofmt -l internal/api/projects.go internal/api/projects_test.go</automated>
  </verify>
  <done>`go test ./internal/api/` exits 0 (new test + all existing pass); `go build ./...` exits 0; gofmt prints nothing for the changed Go files. PATCH with a bogus repo returns 200 with the host-correct verify_state; description-only and unlink PATCHes carry no verify_state field.</done>
</task>

<task type="auto">
  <name>Task 3: Add optional verify_state to the Project TS interface (type-only, no behavior, no rebuild)</name>
  <files>web/src/api/types.ts</files>
  <action>
    In web/src/api/types.ts, add an optional field to the `Project` interface so the
    field the server now returns is typed (the dialog currently reads it via an inline
    `as { verify_state?: string }` cast). Add after `github_repo`:
    ```ts
      // Transient advisory returned ONLY by PATCH /api/projects/{id} when a linked
      // repo could not be verified (D-11): "no_gh" | "unverifiable". Never persisted,
      // absent on GET/list. Optional so list/create rows remain valid.
      verify_state?: string;
    ```
    This is a pure type addition — NO behavior change and NO SPA rebuild is required
    (ProjectSettingsDialog.tsx is already wired and is NOT modified). Do not edit any
    other frontend file.
  </action>
  <verify>
    <automated>cd /home/jordi/workspace/github/kangent && grep -q "verify_state?: string" web/src/api/types.ts && echo TYPE-OK</automated>
  </verify>
  <done>`Project` interface includes `verify_state?: string`; no other frontend file changed; backend tests/build from Task 2 still green.</done>
</task>

</tasks>

<verification>
- `go test ./internal/api/` exits 0 (TestUpdateProjectVerifyState + all existing project/github tests pass).
- `go build ./...` exits 0.
- `gofmt -l internal/api/projects.go internal/api/projects_test.go` prints nothing.
- No DB migration, no scanProject change, no Project JSON-tag change → GET /api/projects and POST responses are byte-for-byte unchanged.
- ProjectSettingsDialog.tsx is unmodified; it already consumes updated.verify_state ("no_gh" → gh-degraded advisory, "unverifiable" → soft-verify advisory, keeps dialog open).
</verification>

<success_criteria>
- PATCH /api/projects/{id} with a syntactically-valid but unverifiable github_repo returns 200 AND a verify_state advisory: "unverifiable" on a gh-present host, "no_gh" on a gh-absent host.
- PATCH with only description (or only name), or with github_repo:"" (unlink), returns 200 with NO verify_state field.
- A syntactically invalid github_repo still hard-blocks with 400 and the canonical error copy (unchanged).
- The change is backend-only plus a one-line TS type addition; no SPA rebuild and no frontend behavior change.
</success_criteria>

<output>
After completion, create `.planning/quick/260613-osu-warn-when-a-linked-github-repo-cannot-be/260613-osu-SUMMARY.md`
</output>
