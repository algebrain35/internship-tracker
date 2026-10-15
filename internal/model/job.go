package model

import "time"

// Source identifies which ATS the job came from.
type Source string

const (
	SourceGreenhouse Source = "greenhouse"
	SourceLever      Source = "lever"
	SourceWorkday    Source = "workday"
)

// WorkplaceType captures the remote/hybrid/onsite nature of a role.
type WorkplaceType string

const (
	WorkplaceOnSite  WorkplaceType = "on-site"
	WorkplaceRemote  WorkplaceType = "remote"
	WorkplaceHybrid  WorkplaceType = "hybrid"
	WorkplaceUnknown WorkplaceType = "unknown"
)

// Job is the canonical DTO shared across all pipeline stages.
// Every ATS-specific raw struct maps into this shape.
type Job struct {
	// --- Identity ---
	ID        string    // Stable dedup key: "{source}:{raw_id}"
	Source    Source    // Which ATS this came from
	FetchedAt time.Time // When our crawler retrieved it

	// --- Core fields (present in all sources) ---
	Title      string
	Company    string
	ApplyURL   string

	// --- Location ---
	// Raw string as returned by the ATS (e.g. "Toronto, ON", "Canada (Remote)")
	LocationRaw   string
	WorkplaceType WorkplaceType

	// --- Classification ---
	// Department / team name (e.g. "Engineering", "Software Development")
	Department string
	// Commitment / employment type (e.g. "Internship", "Co-op", "Full-time")
	Commitment string

	// --- Content ---
	Description string // Full HTML or plain-text job description

	// --- Dates ---
	// PostedAt is the ATS-reported creation/publish date.
	// May be zero if the source doesn't expose it (e.g. Workday).
	PostedAt  time.Time
	UpdatedAt time.Time
}
