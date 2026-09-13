package gitops

import (
	"fmt"
	"strings"
)

type SafetyCheck struct {
	MaxFilesChanged int
	RequireTests    bool
	AutoPush        bool
}

func DefaultSafety() SafetyCheck {
	return SafetyCheck{
		MaxFilesChanged: 10,
		RequireTests:    true,
		AutoPush:        false,
	}
}

type SafetyViolation struct {
	Check   string
	Message string
}

var scopeBudgets = map[string]struct {
	MaxFiles int
	MaxSteps int
}{
	"BUG":      {MaxFiles: 8, MaxSteps: 12},
	"FEATURE":  {MaxFiles: 25, MaxSteps: 40},
	"REFACTOR": {MaxFiles: 15, MaxSteps: 20},
	"CHORE":    {MaxFiles: 10, MaxSteps: 15},
	"PREP":     {MaxFiles: 10, MaxSteps: 15},
}

type ScopeBudget struct {
	MaxFiles int
	MaxSteps int
}

func GetScopeBudget(ticketType string) (ScopeBudget, bool) {
	b, ok := scopeBudgets[strings.ToUpper(ticketType)]
	if !ok {
		return ScopeBudget{}, false
	}
	return ScopeBudget{MaxFiles: b.MaxFiles, MaxSteps: b.MaxSteps}, true
}

func (s SafetyCheck) ValidateScope(ticketType string, planStepCount int, uniqueFiles []string) []SafetyViolation {
	var violations []SafetyViolation

	budget, ok := scopeBudgets[strings.ToUpper(ticketType)]
	if !ok {
		budget = scopeBudgets["CHORE"]
	}

	if len(uniqueFiles) > budget.MaxFiles {
		violations = append(violations, SafetyViolation{
			Check:   "scope_files",
			Message: fmt.Sprintf("%s ticket targets %d files (budget: %d)", ticketType, len(uniqueFiles), budget.MaxFiles),
		})
	}

	if planStepCount > budget.MaxSteps {
		violations = append(violations, SafetyViolation{
			Check:   "scope_steps",
			Message: fmt.Sprintf("%s ticket has %d plan steps (budget: %d)", ticketType, planStepCount, budget.MaxSteps),
		})
	}

	return violations
}

func (s SafetyCheck) Validate(g *GitOps) ([]SafetyViolation, error) {
	var violations []SafetyViolation

	files, err := g.FilesChanged()
	if err != nil {
		return nil, fmt.Errorf("checking files changed: %w", err)
	}

	if len(files) > s.MaxFilesChanged {
		violations = append(violations, SafetyViolation{
			Check:   "max_files_changed",
			Message: fmt.Sprintf("changed %d files (max %d)", len(files), s.MaxFilesChanged),
		})
	}

	return violations, nil
}
