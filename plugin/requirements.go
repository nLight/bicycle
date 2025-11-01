package plugin

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// Requirement represents a single requirement check
type Requirement struct {
	// Name is a short identifier for the requirement
	Name string

	// Description explains what the requirement checks
	Description string

	// CheckFunc performs the actual check
	CheckFunc func(ctx context.Context) error

	// Required indicates if this requirement must pass
	// If false, failures are logged as warnings
	Required bool
}

// RequirementChecker validates a set of requirements
type RequirementChecker struct {
	requirements []Requirement
	pluginName   string
}

// NewRequirementChecker creates a new requirement checker
func NewRequirementChecker(pluginName string) *RequirementChecker {
	return &RequirementChecker{
		requirements: make([]Requirement, 0),
		pluginName:   pluginName,
	}
}

// Add adds a requirement to check
func (rc *RequirementChecker) Add(req Requirement) {
	rc.requirements = append(rc.requirements, req)
}

// AddRequired adds a required requirement
func (rc *RequirementChecker) AddRequired(name, description string, checkFunc func(ctx context.Context) error) {
	rc.Add(Requirement{
		Name:        name,
		Description: description,
		CheckFunc:   checkFunc,
		Required:    true,
	})
}

// AddOptional adds an optional requirement
func (rc *RequirementChecker) AddOptional(name, description string, checkFunc func(ctx context.Context) error) {
	rc.Add(Requirement{
		Name:        name,
		Description: description,
		CheckFunc:   checkFunc,
		Required:    false,
	})
}

// Check runs all requirement checks
// Returns an error if any required check fails
func (rc *RequirementChecker) Check(ctx context.Context) error {
	if len(rc.requirements) == 0 {
		return nil
	}

	log.Printf("[%s] Checking %d requirement(s)...", rc.pluginName, len(rc.requirements))

	var errors []string
	var warnings []string

	for _, req := range rc.requirements {
		if err := req.CheckFunc(ctx); err != nil {
			msg := fmt.Sprintf("%s: %v", req.Name, err)

			if req.Required {
				errors = append(errors, msg)
				log.Printf("[%s] ✗ Required check failed: %s", rc.pluginName, msg)
			} else {
				warnings = append(warnings, msg)
				log.Printf("[%s] ⚠ Optional check failed: %s", rc.pluginName, msg)
			}
		} else {
			log.Printf("[%s] ✓ %s", rc.pluginName, req.Name)
		}
	}

	// Log warnings (non-blocking)
	if len(warnings) > 0 {
		log.Printf("[%s] %d warning(s) - plugin may have reduced functionality", rc.pluginName, len(warnings))
	}

	// Return error if any required checks failed
	if len(errors) > 0 {
		return fmt.Errorf("requirement check(s) failed: %s", strings.Join(errors, "; "))
	}

	log.Printf("[%s] All required checks passed", rc.pluginName)
	return nil
}

// Common requirement check functions

// RequireMode creates a requirement that checks for a specific mode
func RequireMode(requiredMode Mode) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		mode, ok := ctx.Value("mode").(Mode)
		if !ok {
			return fmt.Errorf("mode not set in context")
		}
		if mode != requiredMode {
			return fmt.Errorf("requires %s mode, got %s", requiredMode, mode)
		}
		return nil
	}
}

// RequireEnvVar creates a requirement that checks for an environment variable
func RequireEnvVar(name string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// Access via context to allow for testing
		if val := ctx.Value(fmt.Sprintf("env:%s", name)); val != nil {
			if str, ok := val.(string); ok && str != "" {
				return nil
			}
		}

		// Fallback to actual environment (will be implemented with os.Getenv)
		// For now, we'll check the context
		return fmt.Errorf("environment variable %s not set", name)
	}
}

// RequireAny creates a requirement that passes if any of the given checks pass
func RequireAny(checks ...func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		var errors []string
		for _, check := range checks {
			if err := check(ctx); err == nil {
				return nil // At least one check passed
			} else {
				errors = append(errors, err.Error())
			}
		}
		return fmt.Errorf("all checks failed: %s", strings.Join(errors, "; "))
	}
}

// RequireAll creates a requirement that passes only if all checks pass
func RequireAll(checks ...func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		for _, check := range checks {
			if err := check(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}
