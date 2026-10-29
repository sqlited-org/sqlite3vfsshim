package vfs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadableExtension builds examples/memvfs-ext as a SQLite loadable
// extension and loads it into the sqlite3 command line tool. It needs the Go
// workspace (go.work) at the repository root.
func TestLoadableExtension(t *testing.T) {
	cli, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 CLI not installed")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	root, _ := filepath.Abs("../..")
	if _, err := os.Stat(filepath.Join(root, "go.work")); err != nil {
		t.Skip("go.work not found; run from the repository checkout")
	}
	so := filepath.Join(t.TempDir(), "memvfs.so")

	build := exec.Command(goBin, "build", "-tags", "SQLITE3VFS_LOADABLE_EXT",
		"-buildmode=c-shared", "-o", so, "./memvfs-ext")
	build.Dir = filepath.Join(root, "examples")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building extension: %v\n%s", err, out)
	}

	run := exec.Command(cli,
		"-cmd", ".load "+strings.TrimSuffix(so, ".so"),
		"-cmd", ".open file:ext.db?vfs=memvfs",
		"-cmd", "create table t(v); insert into t values (41),(42);",
		"-cmd", "select sum(v) from t;")
	run.Stdin = strings.NewReader("")
	var stderr strings.Builder
	run.Stderr = &stderr
	out, err := run.Output()
	if err != nil {
		t.Fatalf("sqlite3: %v\nstderr: %s", err, stderr.String())
	}
	if strings.TrimSpace(string(out)) != "83" {
		t.Errorf("unexpected output %q (stderr: %s)", out, stderr.String())
	}
}
