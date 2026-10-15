package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"internship-tracker/internal/model"
)

// workdayCompany holds everything needed to query a single Workday tenant.
type workdayCompany struct {
	displayName string
	tenant      string // subdomain, e.g. "td" in td.wd3.myworkdayjobs.com
	instance    string // "wd3", "wd5", etc.
	site        string // site path segment, e.g. "TD_Bank_Careers"
	siteBase    string // override base URL for myworkdaysite.com tenants (optional)
}

// workdayCompanies is a fallback list used only when dynamic slug discovery
// (discover.go) returns zero Workday entries. Slugs were verified against
// live Workday boards as of 2025.
var workdayCompanies = []workdayCompany{
	{"RBC", "rbc", "wd3", "RBC-Careers", ""},
	{"TD Bank", "td", "wd3", "TD_Bank_Careers", ""},
	{"Scotiabank", "scotiabank", "wd3", "Scotiabank_Careers", ""},
	{"CIBC", "cibc", "wd5", "CIBC-Careers", ""},
	{"Bell Canada", "bell", "wd3", "bell-careers", ""},
	{"Rogers", "rogers", "wd5", "RogersCareers", ""},
	{"CGI", "cgi", "wd3", "CGI-Careers", ""},
	{"Manulife", "manulife", "wd3", "MFC", ""},
}

const workdayPageSize = 20

// RunWorkday fetches Canadian internship postings from every configured
// Workday tenant and sends them on the jobs channel.
func RunWorkday(ctx context.Context, jobs chan<- model.Job, companies []workdayCompany) error {
	client := &http.Client{Timeout: 30 * time.Second}

	for _, company := range companies {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fetched, err := fetchWorkday(ctx, client, company)
		if err != nil {
			return fmt.Errorf("workday[%s]: %w", company.displayName, err)
		}

		for _, job := range fetched {
			select {
			case jobs <- job:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}

// fetchWorkday retrieves all internship postings for a single Workday tenant,
// paginating until all results are consumed.
func fetchWorkday(ctx context.Context, client *http.Client, company workdayCompany) ([]model.Job, error) {
	baseURL := company.siteBase
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s.%s.myworkdayjobs.com",
			company.tenant, company.instance)
	}
	endpoint := fmt.Sprintf("%s/wday/cxs/%s/%s/jobs",
		baseURL, company.tenant, company.site)

	var results []model.Job
	offset := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		batch, total, err := doWorkdayRequest(ctx, client, endpoint, offset)
		if err != nil {
			return nil, err
		}

		for _, w := range batch {
			// Filter: must look like an internship.
			if !isWorkdayInternship(w) {
				continue
			}
			// Filter: must be Canadian.
			if !isCanadianLocation(w.LocationsText) {
				continue
			}
			// ExternalPath is relative to the careers site root.
			// Some tenants return it as /job/Location/Title (no site prefix),
			// others return /SITE/job/Location/Title (site already included).
			// Detect which form and prepend the site only when absent.
			path := w.ExternalPath
			if !strings.HasPrefix(path, "/"+company.site) {
				path = "/" + company.site + path
			}
			applyURL := baseURL + path
			results = append(results, w.ToJob(company.displayName, applyURL))
		}

		offset += len(batch)
		if offset >= total || len(batch) == 0 {
			break
		}
	}

	return results, nil
}

// doWorkdayRequest performs a single paginated POST to the Workday CXS endpoint.
func doWorkdayRequest(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	offset int,
) ([]model.WorkdayJob, int, error) {

	reqBody := model.WorkdayJobSearchRequest{
		SearchText: "intern OR internship OR co-op OR coop",
		Offset:     offset,
		Limit:      workdayPageSize,
		AnonymousShoppingCartReqd: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// 4xx errors mean the tenant/site slug is wrong or the endpoint moved.
	// Treat as empty rather than fatal so other companies still run.
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return nil, 0, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var result model.WorkdayJobSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, 0, fmt.Errorf("decode: %w", err)
	}

	return result.JobPostings, result.Total, nil
}

// isWorkdayInternship returns true if the job title contains internship keywords.
func isWorkdayInternship(w model.WorkdayJob) bool {
	return containsAny(strings.ToLower(w.Title), internshipKeywords)
}
