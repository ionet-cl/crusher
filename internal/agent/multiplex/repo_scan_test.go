package multiplex

import (
	"context"
	"testing"
)

// TestRepoScannerWithRealPath tests scanning the actual Crusher repo.
func TestRepoScannerWithRealPath(t *testing.T) {
	ctx := context.Background()
	scanner := NewRepoScanner()

	// Scan the actual Crusher repo root
	repo := RepoSpec{
		ID:   "crusher",
		Root: "/mnt/Workspace/Desarrollo/referentes-agenticos/crusher",
		Type: RepoTypeLocal,
	}

	contents, err := scanner.Scan(ctx, repo)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	t.Logf("Found %d files in %d modules", len(contents.Files), len(contents.Modules))

	// Show first few files
	for i, f := range contents.Files {
		if i >= 10 {
			t.Logf("... and %d more files", len(contents.Files)-10)
			break
		}
		t.Logf("  File: %s", f)
	}

	// Show modules
	for _, m := range contents.Modules {
		t.Logf("  Module: %s (%d files)", m.Name, len(m.Files))
	}

	if len(contents.Files) == 0 {
		t.Error("No files found")
	}

	if len(contents.Modules) == 0 {
		t.Error("No modules found")
	}
}

// TestIntentFromRepoContents demonstrates how to use the scanner output.
func TestIntentFromRepoContents(t *testing.T) {
	ctx := context.Background()
	scanner := NewRepoScanner()

	repo := RepoSpec{
		ID:   "test-repo",
		Root: ".",
		Type: RepoTypeLocal,
	}

	contents, err := scanner.Scan(ctx, repo)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Create intents for module-based partitioning
	intents := IntentFromRepoContents(contents, "test-task", "Analyze code and find issues", "module")

	t.Logf("Created %d intents from %d files in %d modules",
		len(intents), len(contents.Files), len(contents.Modules))

	for i, intent := range intents {
		if i >= 5 {
			t.Logf("... and %d more intents", len(intents)-5)
			break
		}
		t.Logf("  Intent %d: role=%s, goal=%s, resources=%d",
			i+1, intent.Role, intent.Goal, len(intent.Resources))
	}
}
