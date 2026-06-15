package github

import "context"

// This file exposes the package-level exec seams (cloneRunner, validateRunner)
// to tests in OTHER packages (notably internal/api's create-by-repo tests),
// which cannot reach the unexported vars directly. The setters return a restore
// func so a test can defer-restore the production behavior. They are inert in
// production — nothing but test code ever calls them.

// SetCloneRunnerForTest overrides Clone's exec seam and returns a restore func.
// The fake receives (ctx, ref, dest) and returns (trimmedStderr, err), matching
// realClone's contract: a non-nil err makes Clone os.RemoveAll(dest) and surface
// the stderr; nil means success.
func SetCloneRunnerForTest(fake func(ctx context.Context, ref, dest string) (string, error)) (restore func()) {
	prev := cloneRunner
	cloneRunner = fake
	return func() { cloneRunner = prev }
}

// SetValidateRunnerForTest overrides ValidateRepo's gh-verification leg and
// returns a restore func. The fake receives the already-canonicalized owner/name
// and returns (canonical, verified, err) exactly as ghValidate would. It is only
// reached when Available() is true, so pair it with SetAvailableForTest(true)
// to drive the verified path deterministically off any host.
func SetValidateRunnerForTest(fake func(ctx context.Context, parsed string) (canonical string, verified bool, err error)) (restore func()) {
	prev := validateRunner
	validateRunner = fake
	return func() { validateRunner = prev }
}

// SetDescriptionRunnerForTest overrides RepoDescription's gh-read leg and
// returns a restore func. The fake receives the already-canonicalized
// owner/name and returns the description string exactly as ghDescription would
// (degrade-don't-break: it returns "" — never an error). It is only reached
// when Available() is true, so pair it with SetAvailableForTest(true) to drive
// the captured-description path deterministically off any host.
func SetDescriptionRunnerForTest(fake func(ctx context.Context, canonical string) string) (restore func()) {
	prev := descriptionRunner
	descriptionRunner = fake
	return func() { descriptionRunner = prev }
}

// SetAvailableForTest overrides Available's PATH-lookup seam and returns a
// restore func, so tests can force gh present/absent without touching the host
// PATH (the create-by-repo gh-absent reject and verified paths both rely on it).
func SetAvailableForTest(available bool) (restore func()) {
	prev := availableRunner
	availableRunner = func() bool { return available }
	return func() { availableRunner = prev }
}
