package crawler

import (
	"context"
	"log"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"internship-tracker/internal/model"
)

const greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true"

// greenhouseCompanies is the hardcoded list of board tokens to crawl.
// Replace with companies.yaml loading when ready.
var greenhouseCompanies = []struct {
	slug        string // Greenhouse board token (appears in the URL)
	displayName string // Human-readable company name shown in the UI
}{
	
	// Shopify uses a custom ATS (internships.shopify.com) — not on Greenhouse.
	{"wealthsimple", "Wealthsimple"},
	{"caseware", "CaseWare"},
	{"hootsuite", "Hootsuite"},
	{"clio", "Clio"},
	{"d2l", "D2L"},
	{"tailscale", "Tailscale"},
	{"lightspeed", "Lightspeed"},
	{"financeit", "Financeit"},
	{"ritual", "Ritual"},
	{"snapcommerce", "Snapcommerce"},
}

// internshipKeywords are matched (case-insensitive) against job titles and
// department names to identify internship / co-op postings.
var internshipKeywords = []string{
	"intern", "internship", "co-op", "coop", "co op", "student",
}

// RunGreenhouse fetches all Canadian internship postings from every configured
// Greenhouse company and sends them on the jobs channel.
//
// It respects ctx cancellation — if the shared errgroup context is cancelled
// (because another crawler failed), this function stops at its next HTTP call.
func RunGreenhouse(ctx context.Context, jobs chan<- model.Job, companies []companySlug) error {
	client := &http.Client{Timeout: 30 * time.Second}

	for _, company := range companies {
		// Check for cancellation before each company fetch.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		time.Sleep(300 * time.Millisecond)
		fetched, err := fetchGreenhouse(ctx, client, company.slug, company.displayName)
		if err != nil {
			log.Printf("greenhouse[%s]: skipping — %v", company.slug, err)
			continue
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

// fetchGreenhouse retrieves and filters jobs for a single Greenhouse board.
func fetchGreenhouse(ctx context.Context, client *http.Client, slug, displayName string) ([]model.Job, error) {
	url := fmt.Sprintf(greenhouseBaseURL, slug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Board token doesn't exist or company left Greenhouse — skip silently.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw model.GreenhouseJobList
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	var results []model.Job
	for _, g := range raw.Jobs {
		if !isGreenhouseInternship(g) {
			continue
		}
		if !isCanadianLocation(g.Location.Name) {
			// Also check office locations.
			if !hasCanadianOffice(g.Offices) {
				continue
			}
		}
		results = append(results, g.ToJob(displayName))
	}

	return results, nil
}

// isGreenhouseInternship returns true if the job looks like an internship
// based on its title or department names.
func isGreenhouseInternship(g model.GreenhouseJob) bool {
	haystack := strings.ToLower(g.Title)
	for _, d := range g.Departments {
		haystack += " " + strings.ToLower(d.Name)
	}
	return containsAny(haystack, internshipKeywords)
}

// hasCanadianOffice returns true if any office location mentions Canada.
func hasCanadianOffice(offices []model.GreenhouseOffice) bool {
	for _, o := range offices {
		if isCanadianLocation(o.Location) || isCanadianLocation(o.Name) {
			return true
		}
	}
	return false
}

