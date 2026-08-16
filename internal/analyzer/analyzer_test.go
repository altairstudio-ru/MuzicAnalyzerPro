package analyzer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFindAnalyzerDirAbsolute(t *testing.T) {
	// Tests run with cwd = package dir (internal/analyzer). Climb to module root
	// so the relative "analyzer" / "../../analyzer" candidates resolve.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../.."))
	t.Chdir(repoRoot)

	dir := findAnalyzerDir()
	if dir == "" {
		t.Fatal("findAnalyzerDir returned empty")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("findAnalyzerDir = %q, want absolute", dir)
	}
	if !strings.HasSuffix(filepath.Clean(dir), string(filepath.Separator)+"analyzer") {
		t.Fatalf("findAnalyzerDir = %q, want …/analyzer", dir)
	}
	scriptPath := filepath.Join(dir, "analyze.py")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("analyze.py missing at %q: %v", scriptPath, err)
	}

	a := New(nil, "python3")
	if a.Script == "" {
		t.Fatal("New().Script empty")
	}
	if !filepath.IsAbs(a.Script) {
		t.Fatalf("Script = %q, want absolute", a.Script)
	}
	if !strings.HasSuffix(a.Script, filepath.Join("analyzer", "analyze.py")) {
		t.Fatalf("Script = %q, want …/analyzer/analyze.py", a.Script)
	}
	// Must not be the doubled path that broke subprocess runs.
	if strings.Contains(a.Script, filepath.Join("analyzer", "analyzer")) {
		t.Fatalf("Script has doubled analyzer: %q", a.Script)
	}
}
