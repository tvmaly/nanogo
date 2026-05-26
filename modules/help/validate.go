package help

import (
	"fmt"
	"regexp"
	"strings"
)

var topicIDPattern = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9_]+)+$`)

var RequiredSections = []string{"rules", "summary", "procedure", "examples", "failure_modes", "verification"}

type ValidationError struct{ Errors []string }

func (e ValidationError) Error() string { return strings.Join(e.Errors, "; ") }

func validationError(errs []string) error { return ValidationError{Errors: errs} }

func ValidatePack(pack Pack) []string {
	var errs []string
	if pack.SchemaVersion != PackSchemaVersion {
		errs = append(errs, fmt.Sprintf("unsupported pack schema version %q", pack.SchemaVersion))
	}
	seen := map[string]bool{}
	topicIDs := map[string]bool{}
	for _, t := range pack.Topics {
		if seen[t.ID] {
			errs = append(errs, "duplicate topic id "+t.ID)
		}
		seen[t.ID] = true
		topicIDs[t.ID] = true
		if t.SchemaVersion != TopicSchemaVersion {
			errs = append(errs, fmt.Sprintf("%s unsupported topic schema version %q", t.ID, t.SchemaVersion))
		}
		if !topicIDPattern.MatchString(t.ID) {
			errs = append(errs, "invalid topic id "+t.ID)
		}
		if strings.TrimSpace(t.Title) == "" {
			errs = append(errs, t.ID+" title is required")
		}
		if strings.TrimSpace(t.Summary) == "" {
			errs = append(errs, t.ID+" summary is required")
		}
		if strings.TrimSpace(t.LastVerified) == "" {
			errs = append(errs, t.ID+" last_verified is required")
		}
		for _, section := range RequiredSections {
			if strings.TrimSpace(t.Sections[section]) == "" {
				errs = append(errs, fmt.Sprintf("%s missing required section %s", t.ID, section))
			}
		}
		for _, p := range t.SourcePaths {
			if strings.Contains(p, "..") || strings.HasPrefix(p, "/") || strings.TrimSpace(p) == "" {
				errs = append(errs, fmt.Sprintf("%s unsafe source path %q", t.ID, p))
			}
		}
	}
	for _, t := range pack.Topics {
		for _, rel := range t.Related {
			if !topicIDs[rel] {
				errs = append(errs, fmt.Sprintf("%s related topic %s is missing", t.ID, rel))
			}
		}
	}
	for _, root := range pack.RootTopics {
		if !topicIDs[root] {
			errs = append(errs, "root topic "+root+" is missing")
		}
	}
	return errs
}
