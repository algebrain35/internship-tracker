package model

import (
	"fmt"
	"time"
)

// ----------------------------------------------------------------------------
// Greenhouse raw API shapes
// Mirrors: GET https://boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true
// ----------------------------------------------------------------------------

type GreenhouseJobList struct {
	Jobs []GreenhouseJob `json:"jobs"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

type GreenhouseJob struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`

	Location struct {
		Name string `json:"name"`
	} `json:"location"`

	// Content is the full HTML job description; only present when
	// the request includes ?content=true.
	Content string `json:"content"`

	Departments []GreenhouseDepartment `json:"departments"`
	Offices     []GreenhouseOffice     `json:"offices"`

	// Metadata holds custom fields the company chose to expose.
	// Values are untyped — we only read what we need.
	Metadata []GreenhouseMetadata `json:"metadata"`

	UpdatedAt time.Time `json:"updated_at"`
}

type GreenhouseDepartment struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type GreenhouseOffice struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

type GreenhouseMetadata struct {
	ID        int64       `json:"id"`
	Name      string      `json:"name"`
	Value     interface{} `json:"value"`
	ValueType string      `json:"value_type"`
}

// ToJob maps a raw GreenhouseJob into the canonical Job DTO.
// company is the board token / display name supplied by the caller.
func (g GreenhouseJob) ToJob(company string) Job {
	dept := ""
	if len(g.Departments) > 0 {
		dept = g.Departments[0].Name
	}

	return Job{
		ID:            greenhouseID(company, g.ID),
		Source:        SourceGreenhouse,
		FetchedAt:     time.Now().UTC(),
		Title:         g.Title,
		Company:       company,
		ApplyURL:      g.AbsoluteURL,
		LocationRaw:   g.Location.Name,
		WorkplaceType: inferWorkplaceType(g.Location.Name),
		Department:    dept,
		Commitment:    "Internship", // caller pre-filters by internship department
		Description:   g.Content,
		UpdatedAt:     g.UpdatedAt.UTC(),
		// Greenhouse Job Board API does not expose a created_at / posted_at field.
	}
}

func greenhouseID(company string, rawID int64) string {
	return fmt.Sprintf("greenhouse:%s:%d", company, rawID)
}
