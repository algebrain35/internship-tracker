package model

import (
	"fmt"
	"time"
)

// ----------------------------------------------------------------------------
// Workday raw API shapes
// Mirrors: POST https://{tenant}.wd{n}.myworkdayjobs.com/wday/cxs/{tenant}/{site}/jobs
//
// Workday's CXS API is undocumented. Field names are camelCase strings
// returned in the JSON body. Only the fields relevant to our use case
// are captured here.
// ----------------------------------------------------------------------------

// WorkdayJobSearchRequest is the POST body sent to the CXS jobs endpoint.
type WorkdayJobSearchRequest struct {
	SearchText string                 `json:"searchText"`
	Locations  []WorkdayFacetValue    `json:"locations,omitempty"`
	Offset     int                    `json:"offset"`
	Limit      int                    `json:"limit"`
	AnonymousShoppingCartReqd bool    `json:"anonymousShoppingCartReqd"`
}

// WorkdayFacetValue is a generic {id, descriptor} pair used for filter values.
type WorkdayFacetValue struct {
	ID         string `json:"id"`
	Descriptor string `json:"descriptor"`
}

// WorkdayJobSearchResponse is the top-level CXS response envelope.
type WorkdayJobSearchResponse struct {
	Total       int            `json:"total"`
	JobPostings []WorkdayJob   `json:"jobPostings"`
}

// WorkdayJob is a single posting as returned by the CXS search endpoint.
// The detail endpoint returns richer fields (full description, etc.);
// this struct covers what the list endpoint provides.
type WorkdayJob struct {
	// Title is the human-readable job title.
	Title string `json:"title"`

	// ExternalPath is the relative URL path for the job detail page,
	// e.g. "/en-US/mysite/job/Toronto/Software-Engineer-Intern_12345"
	ExternalPath string `json:"externalPath"`

	// PostedOn is a human-readable string like "Posted 3 Days Ago".
	// Workday does not return a machine-readable timestamp in the list response.
	PostedOn string `json:"postedOn"`

	// JobReqID is Workday's internal requisition identifier.
	JobReqID string `json:"jobReqId"`

	// LocationsText is a display string for the job location, e.g. "Toronto, ON, Canada".
	LocationsText string `json:"locationsText"`

	// JobFamilyGroup is the broad job family, e.g. "Information Technology".
	JobFamilyGroup string `json:"jobFamilyGroup"`

	// TimeType is the employment basis, e.g. "Full time", "Part time".
	TimeType string `json:"timeType"`

	// WorkspaceTypes is the remote/hybrid/onsite indicator.
	// Values observed: "On Site", "Remote", "Hybrid".
	WorkspaceTypes []string `json:"workspaceTypes"`
}

// WorkdayJobDetail is the richer struct returned by the individual job detail endpoint.
// Used in a second fetch pass to populate Description.
type WorkdayJobDetail struct {
	JobPostingInfo struct {
		JobReqID    string `json:"jobReqId"`
		Title       string `json:"title"`
		Description string `json:"jobDescription"` // HTML string
		StartDate   string `json:"startDate"`      // ISO date string, may be empty
	} `json:"jobPostingInfo"`
}

// ToJob maps a raw WorkdayJob into the canonical Job DTO.
// baseURL is the company's Workday base URL used to construct the full apply link.
// company is the display name of the employer.
func (w WorkdayJob) ToJob(company, applyURL string) Job {

	return Job{
		ID:            fmt.Sprintf("workday:%s:%s", company, w.JobReqID),
		Source:        SourceWorkday,
		FetchedAt:     time.Now().UTC(),
		Title:         w.Title,
		Company:       company,
		ApplyURL:      applyURL,
		LocationRaw:   w.LocationsText,
		WorkplaceType: workdayWorkplaceType(w.WorkspaceTypes),
		Department:    w.JobFamilyGroup,
		Commitment:    inferCommitment(w.Title),
		// Description is populated in a second pass via WorkdayJobDetail.
		// PostedAt cannot be reliably parsed from "Posted 3 Days Ago" strings;
		// left as zero value until a detail fetch is performed.
	}
}

func workdayWorkplaceType(types []string) WorkplaceType {
	if len(types) == 0 {
		return WorkplaceUnknown
	}
	switch types[0] {
	case "On Site":
		return WorkplaceOnSite
	case "Remote":
		return WorkplaceRemote
	case "Hybrid":
		return WorkplaceHybrid
	default:
		return WorkplaceUnknown
	}
}
