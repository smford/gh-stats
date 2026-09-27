package hook

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.name", "Test User")
	run("config", "user.email", "test@example.com")
	run("config", "commit.gpgsign", "false")

	// Create initial commit on main
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial commit")

	return dir
}

func TestGenerateHookScript(t *testing.T) {
	t.Run("pre-push without fail-on", func(t *testing.T) {
		script, err := GenerateHookScript(HookPrePush, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(script, HookMarker) {
			t.Errorf("script missing HookMarker")
		}
		if !strings.Contains(script, "hook run --type=pre-push") {
			t.Errorf("script missing execution command")
		}
	})

	t.Run("pre-push with fail-on", func(t *testing.T) {
		script, err := GenerateHookScript(HookPrePush, "HIGH")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(script, `--fail-on="HIGH"`) {
			t.Errorf("script missing fail-on flag")
		}
	})

	t.Run("pre-commit", func(t *testing.T) {
		script, err := GenerateHookScript(HookPreCommit, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(script, "hook run --type=pre-commit") {
			t.Errorf("script missing pre-commit execution")
		}
	})

	t.Run("invalid hook type", func(t *testing.T) {
		_, err := GenerateHookScript("post-merge", "")
		if err == nil {
			t.Errorf("expected error for unsupported hook type, got nil")
		}
	})
}

func TestInstallAndUninstall(t *testing.T) {
	repoDir := initTestGitRepo(t)

	// 1. Install pre-push hook
	installedPath, err := Install(Options{
		RepoPath: repoDir,
		HookType: HookPrePush,
		FailOn:   "HIGH",
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("Stat failed on %s: %v", installedPath, err)
	}
	// Check executable bits
	if info.Mode()&0111 == 0 {
		t.Errorf("hook file is not executable: mode %v", info.Mode())
	}

	content, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), HookMarker) {
		t.Errorf("installed hook missing marker header")
	}

	// 2. Re-install over existing gh-stats hook without force (should succeed)
	_, err = Install(Options{
		RepoPath: repoDir,
		HookType: HookPrePush,
		FailOn:   "CRITICAL",
	})
	if err != nil {
		t.Fatalf("Re-install over gh-stats hook should succeed, got: %v", err)
	}

	// 3. Uninstall hook
	err = Uninstall(Options{
		RepoPath: repoDir,
		HookType: HookPrePush,
	})
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if _, err := os.Stat(installedPath); !os.IsNotExist(err) {
		t.Errorf("expected hook to be removed after uninstall")
	}

	// 4. Uninstall when already removed
	err = Uninstall(Options{
		RepoPath: repoDir,
		HookType: HookPrePush,
	})
	if err == nil {
		t.Errorf("expected error uninstalling non-existent hook, got nil")
	}

	// 5. Test overwrite protection on custom hook
	customHook := filepath.Join(repoDir, ".git", "hooks", "pre-push")
	if err := os.WriteFile(customHook, []byte("#!/bin/sh\necho custom\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// Install should fail without force
	_, err = Install(Options{
		RepoPath: repoDir,
		HookType: HookPrePush,
		Force:    false,
	})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected error mentioning --force, got: %v", err)
	}

	// Uninstall should fail without force
	err = Uninstall(Options{
		RepoPath: repoDir,
		HookType: HookPrePush,
		Force:    false,
	})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected error mentioning --force, got: %v", err)
	}

	// Install with force should succeed
	_, err = Install(Options{
		RepoPath: repoDir,
		HookType: HookPrePush,
		Force:    true,
	})
	if err != nil {
		t.Fatalf("Install with force failed: %v", err)
	}
}

func TestRunPrePush(t *testing.T) {
	repoDir := initTestGitRepo(t)

	// In test repo, HEAD is at initial commit. If we compare against HEAD, diff is empty
	err := Run(RunOptions{
		RepoPath: repoDir,
		HookType: HookPrePush,
		BaseRef:  "HEAD",
		Quiet:    true,
	})
	if err != nil {
		t.Fatalf("Run on identical HEAD should pass, got: %v", err)
	}
}

func TestRunPreCommit(t *testing.T) {
	repoDir := initTestGitRepo(t)

	// No staged changes
	err := Run(RunOptions{
		RepoPath: repoDir,
		HookType: HookPreCommit,
		Quiet:    true,
	})
	if err != nil {
		t.Fatalf("Run on empty staged changes should pass, got: %v", err)
	}

	// Stage a new file
	newFile := filepath.Join(repoDir, "app.go")
	if err := os.WriteFile(newFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "app.go")
	cmd.Dir = repoDir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	// Run pre-commit with high tolerance threshold (CRITICAL) -> passes
	err = Run(RunOptions{
		RepoPath: repoDir,
		HookType: HookPreCommit,
		FailOn:   "CRITICAL",
		Quiet:    true,
	})
	if err != nil {
		t.Fatalf("pre-commit with CRITICAL threshold should pass, got: %v", err)
	}
}
