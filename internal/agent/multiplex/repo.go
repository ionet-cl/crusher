// Package multiplex provides multi-agent communication primitives.
package multiplex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/fsext"
)

// RepoType specifies the type of repository.
type RepoType string

const (
	RepoTypeLocal RepoType = "local"
	RepoTypeGit  RepoType = "git"
)

// RepoSpec describes a repository to process.
type RepoSpec struct {
	ID   string // unique identifier
	Root string // absolute path or git URL
	Type RepoType
}

// RepoContents contains the scanned contents of a repository.
type RepoContents struct {
	Repo    RepoSpec
	Files   []string // absolute paths to files
	Modules []Module // grouped files by module/directory
}

// Module represents a directory/module within a repo.
type Module struct {
	Name   string
	Path   string
	Files  []string
}

// RepoScanner scans repositories and finds relevant files.
type RepoScanner struct {
	MaxFiles int
	Patterns []string
}

// DefaultRepoPatterns returns common code file patterns (recursive).
func DefaultRepoPatterns() []string {
	return []string{"**/*.go", "**/*.ts", "**/*.tsx", "**/*.js", "**/*.jsx", "**/*.py", "**/*.rs", "**/*.java", "**/*.c", "**/*.cpp", "**/*.h"}
}

// NewRepoScanner creates a new repository scanner.
func NewRepoScanner() *RepoScanner {
	return &RepoScanner{
		MaxFiles: 10000,
		Patterns: DefaultRepoPatterns(),
	}
}

// Scan scans a repository and returns its contents.
func (rs *RepoScanner) Scan(ctx context.Context, repo RepoSpec) (*RepoContents, error) {
	if repo.Type == "" {
		repo.Type = RepoTypeLocal
	}

	switch repo.Type {
	case RepoTypeLocal:
		return rs.scanLocal(ctx, repo)
	case RepoTypeGit:
		// TODO: implement git clone and scan
		return nil, fmt.Errorf("git repos not yet implemented")
	default:
		return nil, fmt.Errorf("unknown repo type: %s", repo.Type)
	}
}

// scanLocal scans a local directory.
func (rs *RepoScanner) scanLocal(ctx context.Context, repo RepoSpec) (*RepoContents, error) {
	root := repo.Root
	if !filepath.IsAbs(root) {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path: %w", err)
		}
		root = absRoot
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("failed to stat repo root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repo root is not a directory: %s", root)
	}

	var files []string

	// Use fsext.GlobGitignoreAware for each pattern
	for _, pattern := range rs.Patterns {
		// Check context periodically
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		matches, _, err := fsext.GlobGitignoreAware(pattern, root, rs.MaxFiles-len(files))
		if err != nil {
			continue // skip pattern errors
		}
		files = append(files, matches...)
	}

	// Remove duplicates
	files = uniqueStrings(files)

	// Group files by module
	modules := groupByModule(files)

	return &RepoContents{
		Repo:    repo,
		Files:   files,
		Modules: modules,
	}, nil
}

// uniqueStrings removes duplicates from a slice.
func uniqueStrings(list []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// groupByModule groups files by their parent directory/module.
func groupByModule(files []string) []Module {
	moduleMap := make(map[string]*Module)

	for _, file := range files {
		dir := filepath.Dir(file)
		if dir == "." || dir == "" {
			dir = "root"
		}
		dir = filepath.Clean(dir)

		if _, exists := moduleMap[dir]; !exists {
			moduleMap[dir] = &Module{
				Name:  filepath.Base(dir),
				Path:  dir,
				Files: []string{},
			}
		}
		moduleMap[dir].Files = append(moduleMap[dir].Files, file)
	}

	var modules []Module
	for _, m := range moduleMap {
		modules = append(modules, *m)
	}

	return modules
}

// ScanMultiple scans multiple repositories.
func (rs *RepoScanner) ScanMultiple(ctx context.Context, repos []RepoSpec) ([]*RepoContents, error) {
	results := make([]*RepoContents, 0, len(repos))

	for _, repo := range repos {
		contents, err := rs.Scan(ctx, repo)
		if err != nil {
			fmt.Printf("Warning: failed to scan repo %s: %v\n", repo.ID, err)
			continue
		}
		results = append(results, contents)
	}

	return results, nil
}

// IntentFromRepoContents converts repo contents to intents for multiplexing.
func IntentFromRepoContents(contents *RepoContents, taskID, taskDescription string, splitBy string) []Intent {
	var intents []Intent

	switch splitBy {
	case "module":
		for i, mod := range contents.Modules {
			intent := Intent{
				ID:        fmt.Sprintf("%s-%s-%d", contents.Repo.ID, mod.Name, i),
				TaskID:    taskID,
				Role:      deriveRoleFromTask(taskDescription),
				Goal:      fmt.Sprintf("[%s:%s] %s", contents.Repo.ID, mod.Name, taskDescription),
				Resources: mod.Files,
				Input:     map[string]string{"repo": contents.Repo.ID, "module": mod.Path},
				Priority:  i,
			}
			intents = append(intents, intent)
		}
	case "file":
		for i, file := range contents.Files {
			intent := Intent{
				ID:        fmt.Sprintf("%s-file-%d", contents.Repo.ID, i),
				TaskID:    taskID,
				Role:      deriveRoleFromTask(taskDescription),
				Goal:      fmt.Sprintf("[%s] %s", contents.Repo.ID, taskDescription),
				Resources: []string{file},
				Input:     map[string]string{"repo": contents.Repo.ID},
				Priority:  i,
			}
			intents = append(intents, intent)
		}
	default:
		intent := Intent{
			ID:        fmt.Sprintf("%s-all", contents.Repo.ID),
			TaskID:    taskID,
			Role:      deriveRoleFromTask(taskDescription),
			Goal:      fmt.Sprintf("[%s] %s", contents.Repo.ID, taskDescription),
			Resources: contents.Files,
			Input:     map[string]string{"repo": contents.Repo.ID},
			Priority:  0,
		}
		intents = append(intents, intent)
	}

	return intents
}

// deriveRoleFromTask infers the agent role from task description.
func deriveRoleFromTask(task string) Role {
	lower := strings.ToLower(task)

	if strings.Contains(lower, "refactor") ||
		strings.Contains(lower, "change") ||
		strings.Contains(lower, "modify") {
		return RoleEditor
	}
	if strings.Contains(lower, "review") ||
		strings.Contains(lower, "check") ||
		strings.Contains(lower, "validate") {
		return RoleReviewer
	}
	if strings.Contains(lower, "fetch") ||
		strings.Contains(lower, "get") ||
		strings.Contains(lower, "download") {
		return RoleFetcher
	}

	return RoleAnalyzer
}
