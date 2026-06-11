package diff

// Table tests over VERBATIM machine-format fixtures captured in 05-RESEARCH.md
// (git 2.43.0). The two parsers are pure functions — no git, no temp dirs — so
// every edge git emits (rename desync, binary, no-newline, mode-only, deleted,
// new) is asserted against a fixed string rather than a live repo.

import (
	"reflect"
	"testing"
)

func TestParseNumstatZ(t *testing.T) {
	// Verified record formats (05-RESEARCH.md §Verified output formats):
	//   plain : added<TAB>deleted<TAB><path><NUL>
	//   binary: -<TAB>-<TAB><path><NUL>
	//   rename: added<TAB>deleted<TAB><NUL><old><NUL><new><NUL>   (EMPTY path)
	// The rename record carries an empty path field then TWO NUL tokens; a
	// naive fields[2]=path parser desyncs everything after it (Pitfall 2).
	out := "1\t0\tkeep.txt\x00" +
		"-\t-\ttracked.bin\x00" +
		"0\t0\t\x00oldname.txt\x00newname.txt\x00" +
		"5\t2\tafter-rename.go\x00"

	got := parseNumstatZ(out)
	want := []numstatRecord{
		{Added: 1, Deleted: 0, Binary: false, Path: "keep.txt", OldPath: ""},
		{Added: 0, Deleted: 0, Binary: true, Path: "tracked.bin", OldPath: ""},
		{Added: 0, Deleted: 0, Binary: false, Path: "newname.txt", OldPath: "oldname.txt"},
		// The record AFTER the rename must keep its correct path/stats — proves
		// the rename's two extra NUL tokens were consumed, not left to desync.
		{Added: 5, Deleted: 2, Binary: false, Path: "after-rename.go", OldPath: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNumstatZ desync:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestParseNumstatZEmpty(t *testing.T) {
	if got := parseNumstatZ(""); len(got) != 0 {
		t.Fatalf("parseNumstatZ(\"\") = %#v, want empty", got)
	}
}

func TestParsePatchModified(t *testing.T) {
	// A plain modified file with one hunk. Note the `\ No newline at end of
	// file` marker — it must NEVER become a content line.
	patch := "diff --git a/app.txt b/app.txt\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/app.txt\n" +
		"+++ b/app.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		" context line\n" +
		"-old line\n" +
		"+new line\n" +
		" trailing\n" +
		"\\ No newline at end of file\n"

	files := parsePatch(patch)
	if len(files) != 1 {
		t.Fatalf("parsePatch returned %d sections, want 1", len(files))
	}
	f := files[0]
	if f.Path != "app.txt" {
		t.Errorf("Path = %q, want app.txt", f.Path)
	}
	if f.Status != "modified" {
		t.Errorf("Status = %q, want modified", f.Status)
	}
	if f.Binary {
		t.Errorf("Binary = true, want false")
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("Hunks = %d, want 1", len(f.Hunks))
	}
	h := f.Hunks[0]
	if h.Header != "@@ -1,3 +1,3 @@" {
		t.Errorf("hunk header = %q, want verbatim @@ -1,3 +1,3 @@", h.Header)
	}
	wantLines := []Line{
		{Kind: "context", Text: " context line"},
		{Kind: "del", Text: "-old line"},
		{Kind: "add", Text: "+new line"},
		{Kind: "context", Text: " trailing"},
	}
	if !reflect.DeepEqual(h.Lines, wantLines) {
		t.Errorf("lines:\n got=%#v\nwant=%#v (No-newline marker must be dropped)", h.Lines, wantLines)
	}
}

func TestParsePatchNewFile(t *testing.T) {
	patch := "diff --git a/added.txt b/added.txt\n" +
		"new file mode 100644\n" +
		"index 0000000..3333333\n" +
		"--- /dev/null\n" +
		"+++ b/added.txt\n" +
		"@@ -0,0 +1,2 @@\n" +
		"+first\n" +
		"+second\n"

	files := parsePatch(patch)
	if len(files) != 1 {
		t.Fatalf("parsePatch returned %d sections, want 1", len(files))
	}
	f := files[0]
	if f.Path != "added.txt" {
		t.Errorf("Path = %q, want added.txt (from +++ b/ when --- is /dev/null)", f.Path)
	}
	if f.Status != "new" {
		t.Errorf("Status = %q, want new (new file mode header)", f.Status)
	}
	if len(f.Hunks) != 1 || len(f.Hunks[0].Lines) != 2 {
		t.Fatalf("want one hunk with 2 add lines, got %#v", f.Hunks)
	}
}

func TestParsePatchDeletedFile(t *testing.T) {
	patch := "diff --git a/gone.txt b/gone.txt\n" +
		"deleted file mode 100644\n" +
		"index 4444444..0000000\n" +
		"--- a/gone.txt\n" +
		"+++ /dev/null\n" +
		"@@ -1,2 +0,0 @@\n" +
		"-was here\n" +
		"-and here\n"

	files := parsePatch(patch)
	if len(files) != 1 {
		t.Fatalf("parsePatch returned %d sections, want 1", len(files))
	}
	f := files[0]
	if f.Path != "gone.txt" {
		t.Errorf("Path = %q, want gone.txt (from --- a/ when +++ is /dev/null)", f.Path)
	}
	if f.Status != "deleted" {
		t.Errorf("Status = %q, want deleted", f.Status)
	}
}

func TestParsePatchRename(t *testing.T) {
	// 100% similarity rename: no hunks, similarity index / rename from / rename
	// to headers, status "renamed" with oldPath set.
	patch := "diff --git a/oldname.txt b/newname.txt\n" +
		"similarity index 100%\n" +
		"rename from oldname.txt\n" +
		"rename to newname.txt\n"

	files := parsePatch(patch)
	if len(files) != 1 {
		t.Fatalf("parsePatch returned %d sections, want 1", len(files))
	}
	f := files[0]
	if f.Status != "renamed" {
		t.Errorf("Status = %q, want renamed", f.Status)
	}
	if f.Path != "newname.txt" {
		t.Errorf("Path = %q, want newname.txt (rename to)", f.Path)
	}
	if f.OldPath == nil || *f.OldPath != "oldname.txt" {
		t.Errorf("OldPath = %v, want oldname.txt (rename from)", f.OldPath)
	}
	if len(f.Hunks) != 0 {
		t.Errorf("Hunks = %d, want 0 for a 100%% similarity rename", len(f.Hunks))
	}
}

func TestParsePatchModeOnly(t *testing.T) {
	// A pure permission change produces NO hunks.
	patch := "diff --git a/script.sh b/script.sh\n" +
		"old mode 100644\n" +
		"new mode 100755\n"

	files := parsePatch(patch)
	if len(files) != 1 {
		t.Fatalf("parsePatch returned %d sections, want 1", len(files))
	}
	f := files[0]
	if f.Path != "script.sh" {
		t.Errorf("Path = %q, want script.sh", f.Path)
	}
	if f.Status != "modified" {
		t.Errorf("Status = %q, want modified (mode-only)", f.Status)
	}
	if len(f.Hunks) != 0 {
		t.Errorf("Hunks = %d, want 0 for a mode-only change", len(f.Hunks))
	}
}

func TestParsePatchBinary(t *testing.T) {
	patch := "diff --git a/logo.png b/logo.png\n" +
		"index 5555555..6666666 100644\n" +
		"Binary files a/logo.png and b/logo.png differ\n"

	files := parsePatch(patch)
	if len(files) != 1 {
		t.Fatalf("parsePatch returned %d sections, want 1", len(files))
	}
	f := files[0]
	if !f.Binary {
		t.Errorf("Binary = false, want true (Binary files ... differ)")
	}
	if f.Path != "logo.png" {
		t.Errorf("Path = %q, want logo.png", f.Path)
	}
	if len(f.Hunks) != 0 {
		t.Errorf("Hunks = %d, want 0 for a binary file", len(f.Hunks))
	}
}

func TestParsePatchMultiFile(t *testing.T) {
	// Multiple sections split on line-anchored "diff --git". A hunk body that
	// happens to contain the text "diff --git" mid-line must NOT split.
	patch := "diff --git a/one.txt b/one.txt\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/one.txt\n" +
		"+++ b/one.txt\n" +
		"@@ -1 +1,2 @@\n" +
		" keep\n" +
		"+added a line mentioning diff --git in prose\n" +
		"diff --git a/two.txt b/two.txt\n" +
		"index 3333333..4444444 100644\n" +
		"--- a/two.txt\n" +
		"+++ b/two.txt\n" +
		"@@ -1 +1 @@\n" +
		"-before\n" +
		"+after\n"

	files := parsePatch(patch)
	if len(files) != 2 {
		t.Fatalf("parsePatch returned %d sections, want 2", len(files))
	}
	if files[0].Path != "one.txt" || files[1].Path != "two.txt" {
		t.Fatalf("paths = %q,%q want one.txt,two.txt", files[0].Path, files[1].Path)
	}
	// The "+added a line mentioning diff --git" content line stays an add line
	// in section one (a "+" prefix is content, not a section boundary).
	if len(files[0].Hunks) != 1 {
		t.Fatalf("file one hunks = %d, want 1", len(files[0].Hunks))
	}
	last := files[0].Hunks[0].Lines[len(files[0].Hunks[0].Lines)-1]
	if last.Kind != "add" {
		t.Errorf("last line of one.txt = %q, want add (the 'diff --git' is content, not a boundary)", last.Kind)
	}
}

func TestParsePatchEmpty(t *testing.T) {
	if got := parsePatch(""); len(got) != 0 {
		t.Fatalf("parsePatch(\"\") = %#v, want empty", got)
	}
}
