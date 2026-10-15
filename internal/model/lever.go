package model

import (
	"fmt"
	"time"
)

// ----------------------------------------------------------------------------
// Lever raw API shapes
// Mirrors: GET https://api.lever.co/v0/postings/{site}?mode=json
// ----------------------------------------------------------------------------

type LeverPosting struct {
	ID   string `json:"id"`
	Text string `json:"text"` // Job title

	Categories LeverCategories `json:"categories"`

	// Description is styled HTML of the main body.
	Description string `json:"description"`
	// DescriptionPlain is plain-text version.
	DescriptionPlain string `json:"descriptionPlain"`

	// Lists holds extra sections like Requirements, Benefits.
	Lists []LeverList `json:"lists"`

	// Additional is closing content (HTML).
	Additional string `json:"additional"`

	HostedURL string `json:"hostedUrl"` // Job listing page URL
	ApplyURL  string `json:"applyUrl"`  // Direct application URL

	// CreatedAt is milliseconds since Unix epoch.
	CreatedAt int64 `json:"createdAt"`

	WorkplaceType string `json:"workplaceType"` // "on-site" | "remote" | "hybrid" | "unspecified"
}

type LeverCategories struct {
	Team       string `json:"team"`
	Department string `json:"department"`
	Location   string `json:"location"`
	// Commitment describes the employment type, e.g. "Internship", "Co-op", "Full-Time".
	Commitment string `json:"commitment"`
	Level      string `json:"level"`
}

type LeverList struct {
	Text    string `json:"text"`    // Section heading (e.g. "Requirements")
	Content string `json:"content"` // Unstyled HTML of list elements
}

// ToJob maps a raw LeverPosting into the canonical Job DTO.
// company is the Lever site name / display name supplied by the caller.
func (l LeverPosting) ToJob(company string) Job {
	dept := l.Categories.Department
	if dept == "" {
		dept = l.Categories.Team
	}

	postedAt := time.Time{}
	if l.CreatedAt > 0 {
		postedAt = time.UnixMilli(l.CreatedAt).UTC()
	}

	return Job{
		ID:            fmt.Sprintf("lever:%s:%s", company, l.ID),
		Source:        SourceLever,
		FetchedAt:     time.Now().UTC(),
		Title:         l.Text,
		Company:       company,
		ApplyURL:      l.ApplyURL,
		LocationRaw:   l.Categories.Location,
		WorkplaceType: leverWorkplaceType(l.WorkplaceType),
		Department:    dept,
		Commitment:    l.Categories.Commitment,
		Description:   l.Description,
		PostedAt:      postedAt,
	}
}

func leverWorkplaceType(raw string) WorkplaceType {
	switch raw {
	case "on-site":
		return WorkplaceOnSite
	case "remote":
		return WorkplaceRemote
	case "hybrid":
		return WorkplaceHybrid
	default:
		return WorkplaceUnknown
	}
}
