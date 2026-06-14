package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cloneRunner is the overridable seam for the actual clone exec, mirroring
// service.go's Config.Runner indirection but at PACKAGE level — Clone is a
// package function (like ValidateRepo), not a Service method, so the seam is a
// package var rather than a struct field. Production points at realClone;
// tests swap in a fake to exercise Clone's success/cleanup logic without a live
// gh or network (the unit-test entry point the plan-02 API atomicity tests
// drive). It returns the trimmed stderr (for the error message) plus the error.
var cloneRunner = realClone

// Clone runs `gh repo clone <ref> <dest>` (host gh auth → private/org repos).
// Contract (host-verified, 14-RESEARCH.md §"Suggested github.Clone signature"):
//
//   - Success is exit 0 ONLY — gh/git exit codes are otherwise unreliable
//     (cli/cli#8845), the same rule as ValidateRepo and worktree.gitRun.
//   - On ANY failure: best-effort os.RemoveAll(dest) (git usually already
//     removed its own partial dir — verified — this covers residue), then
//     return an error whose message is the trimmed stderr (a leading "fatal: "
//     stripped, matching worktree.gitRun), falling back to err.Error() when
//     stderr is empty.
//   - NO short timeout: a large clone legitimately takes minutes (Pitfall 6).
//     The passed ctx is used as-is — never wrapped in a short WithTimeout.
//   - Clones DIRECTLY into the final dest (no temp-dir-then-rename: a rename
//     would break git's internal absolute paths). The caller (plan 02)
//     guarantees dest does not pre-exist non-empty (reattach/dedup checks).
//
// ref is the gh-canonical owner/name from ValidateRepo.
func Clone(ctx context.Context, ref, dest string) error {
	// gh auto-creates parents, but ensure the parent exists for robustness
	// (keeps any future git-clone fallback honest, per research).
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("%s", err.Error())
	}

	stderr, err := cloneRunner(ctx, ref, dest)
	if err != nil {
		// Best-effort cleanup: git usually already removed its partial dir on
		// failure (verified); this covers the residual cases. Ignore the
		// RemoveAll error — we are already on the failure path.
		_ = os.RemoveAll(dest)

		msg := strings.TrimSpace(stderr)
		msg = strings.TrimPrefix(msg, "fatal: ")
		if msg == "" {
			msg = err.Error() // e.g. gh binary missing, context canceled
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// realClone is the production cloneRunner: it shells out to `gh repo clone` with
// an arg array (NEVER `sh -c` — the package invariant; ref is attacker-adjacent
// owner/name input, so no shell). Success is exit 0 only; any nonzero exit
// surfaces as a non-nil error and Clone cleans up. Returns the trimmed stderr
// for the caller's error message.
func realClone(ctx context.Context, ref, dest string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "repo", "clone", ref, dest)
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(errb.String()), err
	}
	return "", nil
}
