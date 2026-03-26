// Package multiplex provides multi-agent communication primitives.
// Design principles:
// - DRY: Intent/Result are the ONLY message types between agents
// - KISS: Plain structs, no business logic
// - SOLID: Each type has single responsibility
// - No AI lore: These are pure data transfer objects
package multiplex

import ()

// Role defines what an agent should do with an intent.
type Role string

const (
	RoleAnalyzer  Role = "analyzer"  // Analyze code/files
	RoleEditor    Role = "editor"     // Modify files
	RoleReviewer  Role = "reviewer"   // Review and validate
	RoleFetcher   Role = "fetcher"    // Fetch external info
	RoleCoordinator Role = "coordinator" // Orchestrate other agents
)

// Intent is the single message type passed from supervisor to sub-agent.
// It contains everything an agent needs to derive its work.
// No AI-specific coupling - this is pure data.
type Intent struct {
	// ID uniquely identifies this intent in the system.
	ID string

	// TaskID groups related intents for aggregation.
	TaskID string

	// Role defines the agent's responsibility.
	Role Role

	// Goal is what the agent should achieve.
	// Agent derives context needed from this.
	Goal string

	// Constraints are hard requirements (must/must not).
	Constraints []Constraint

	// Resources are paths/URIs the agent can access.
	Resources []string

	// Input is optional additional data (file contents, etc).
	Input map[string]string

	// Priority affects execution order (higher = sooner).
	Priority int
}

// Constraint represents a hard requirement.
type Constraint struct {
	Type  ConstraintType
	Value string
}

type ConstraintType string

const (
	ConstraintMust        ConstraintType = "must"
	ConstraintMustNot     ConstraintType = "must_not"
	ConstraintPrefer      ConstraintType = "prefer"
	ConstraintAvoid       ConstraintType = "avoid"
)

// Result is the single message type passed from sub-agent back to supervisor.
// It contains the outcome of the agent's work.
type Result struct {
	// IntentID references the Intent this is a result for.
	IntentID string

	// TaskID groups related results.
	TaskID string

	// Status indicates success/failure.
	Status ResultStatus

	// Output is the primary result content.
	Output string

	// Details for logging/debugging.
	Details []Detail

	// FilesModified lists paths changed by this agent.
	FilesModified []string

	// Error is populated if Status is Failed.
	Error *ResultError
}

type ResultStatus string

const (
	StatusSuccess ResultStatus = "success"
	StatusPartial ResultStatus = "partial" // Completed but with issues
	StatusFailed  ResultStatus = "failed"
	StatusSkipped ResultStatus = "skipped" // Prerequisites not met
)

// Detail is additional context about the result.
type Detail struct {
	Key   string
	Value string
}

// ResultError captures failure information.
type ResultError struct {
	Code    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *ResultError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Message
}

// NewResultError creates a ResultError from a standard error.
func NewResultError(err error) *ResultError {
	if err == nil {
		return nil
	}
	// If it's already a ResultError, return it
	if re, ok := err.(*ResultError); ok {
		return re
	}
	// Otherwise wrap it
	return &ResultError{
		Code:    "UNKNOWN",
		Message: err.Error(),
		Cause:   err,
	}
}
