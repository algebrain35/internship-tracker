package crawler

import (
	"context"
	"log"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"internship-tracker/internal/model"
)

const leverBaseURL = "https://api.lever.co/v0/postings/%s"

// leverCompanies is the hardcoded list of Lever site names to crawl.
// Slugs verified from actual jobs.lever.co URLs in the wild.
var leverCompanies = []struct {
	slug        string
	displayName string
}{
	{"wealthsimple", "Wealthsimple"},  // jobs.lever.co/wealthsimple
	{"1password", "1Password"},        // jobs.lever.co/1password
	{"cohere", "Cohere"},              // jobs.lever.co/cohere
	{"waabi", "Waabi"},                // jobs.lever.co/waabi
	{"intelliware", "Intelliware"},    // jobs.lever.co/intelliware
	{"fullscript", "Fullscript"},      // jobs.lever.co/fullscript
	{"super-com", "Super.com"},        // jobs.lever.co/super-com
	{"coveo", "Coveo"},                // jobs.lever.co/coveo
	{"benchsci", "BenchSci"},          // jobs.lever.co/benchsci
	{"ecobee", "Ecobee"},              // jobs.lever.co/ecobee
}

// canadianLocations are the location strings passed as server-side filters
// to the Lever API. Multiple values are OR'd together.
var canadianLocations = []string{
	"Canada",
	"Toronto",
	"Vancouver",
	"Montreal",
	"Ottawa",
	"Calgary",
	"Remote, Canada",
	"Canada (Remote)",
}

// RunLever fetches Canadian internship postings from every configured
// Lever company and sends them on the jobs channel.
func RunLever(ctx context.Context, jobs chan<- model.Job, companies []companySlug) error {
	client := &http.Client{Timeout: 30 * time.Second}

	for _, company := range companies {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		time.Sleep(500 * time.Millisecond)
		fetched, err := fetchLever(ctx, client, company.slug, company.displayName)
		if err != nil {
			log.Printf("lever[%s]: skipping — %v", company.slug, err)
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

// fetchLever retrieves internship postings for a single Lever site.
func fetchLever(ctx context.Context, client *http.Client, slug, displayName string) ([]model.Job, error) {
	var allPostings []model.LeverPosting

	// Try multiple commitment tags — Canadian companies use different labels.
	for _, commitment := range []string{"Intern", "Internship", "Co-op", "Student"} {
		params := url.Values{}
		params.Set("mode", "json")
		params.Set("commitment", commitment)
		for _, loc := range canadianLocations {
			params.Add("location", loc)
		}

		postings, err := doLeverRequest(ctx, client, slug, params)
		if err != nil {
			return nil, err
		}
		allPostings = append(allPostings, postings...)
	}

	// Also fetch with NO commitment filter — some companies don't tag commitment at all.
	params := url.Values{}
	params.Set("mode", "json")
	for _, loc := range canadianLocations {
		params.Add("location", loc)
	}
	postings, err := doLeverRequest(ctx, client, slug, params)
	if err != nil {
		return nil, err
	}
	allPostings = append(allPostings, postings...)

	// Dedup by Lever ID within this batch, then filter.
	seen := make(map[string]bool)
	var results []model.Job
	for _, p := range allPostings {
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true

		// Must look like an internship.
		if !containsAny(strings.ToLower(p.Text), internshipKeywords) &&
			!containsAny(strings.ToLower(p.Categories.Commitment), internshipKeywords) {
			continue
		}

		// Belt-and-suspenders location check.
		if !isCanadianLocation(p.Categories.Location) {
			continue
		}

		results = append(results, p.ToJob(displayName))
	}

	return results, nil
}

// doLeverRequest performs a single GET to the Lever postings API.
func doLeverRequest(ctx context.Context, client *http.Client, slug string, params url.Values) ([]model.LeverPosting, error) {
	rawURL := fmt.Sprintf(leverBaseURL, slug) + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 404 = slug doesn't exist on Lever, treat as empty.
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Lever returns {"ok":false,"error":"..."} for unknown slugs even with 200.
	if len(body) > 2 && body[0] == '{' {
		return nil, nil // not a JSON array, skip silently
	}

	var postings []model.LeverPosting
	if err := json.Unmarshal(body, &postings); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return postings, nil
}

// isCanadianLocation returns true if the location string refers to Canada.
func isCanadianLocation(location string) bool {
	lower := strings.ToLower(location)
	canadianTerms := []string{
		"canada", "toronto", "vancouver", "montreal", "ottawa",
		"calgary", "edmonton", "winnipeg", "quebec", "ontario",
		"british columbia", "alberta", " on,", " bc,", " qc,",
	}
	return containsAny(lower, canadianTerms)
}
