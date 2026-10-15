package model

import "strings"

// inferWorkplaceType does a best-effort parse of a raw location string.
// Used by sources (like Greenhouse) that don't return a structured workplace type.
func inferWorkplaceType(location string) WorkplaceType {
	lower := strings.ToLower(location)
	switch {
	case strings.Contains(lower, "remote"):
		return WorkplaceRemote
	case strings.Contains(lower, "hybrid"):
		return WorkplaceHybrid
	case location == "" || location == "Anywhere":
		return WorkplaceRemote
	default:
		return WorkplaceOnSite
	}
}

// inferCommitment does a best-effort parse of a job title to determine
// the employment type. Used by sources (like Workday) that don't return
// a structured commitment / employment-type field at the list level.
func inferCommitment(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "intern"):
		return "Internship"
	case strings.Contains(lower, "co-op"), strings.Contains(lower, "coop"):
		return "Co-op"
	case strings.Contains(lower, "co op"):
		return "Co-op"
	default:
		return "Internship" // crawler pre-filters for internship roles
	}
}
