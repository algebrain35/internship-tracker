package crawler

import (
	"context"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Sources are crowdsourced Canadian internship lists on GitHub.
// Their READMEs contain hundreds of verified, current apply URLs
// across Greenhouse, Lever, and Workday — far more reliable than
// any hardcoded slug list.
var readmeSources = []string{
	"https://raw.githubusercontent.com/negarprh/Canadian-Tech-Internships-2026/main/README.md",
	"https://raw.githubusercontent.com/Dannny-Babs/Canadian-Tech-Internships-2025/main/README.md",
}

// Patterns to extract slugs from apply URLs found in the README tables.
var (
	// job-boards.greenhouse.io/SLUG/jobs/ID  or  boards.greenhouse.io/SLUG/jobs/ID
	reGreenhouse = regexp.MustCompile(`(?:job-boards|boards)\.greenhouse\.io/([a-zA-Z0-9_-]+)/jobs`)

	// jobs.lever.co/SLUG/UUID
	reLever = regexp.MustCompile(`jobs\.lever\.co/([a-zA-Z0-9_.-]+)/[a-f0-9-]{36}`)

	// TENANT.wd{N}.myworkdayjobs.com/[en-US/]SITE/job/
	// Fixed: explicitly consume locale prefix before site segment.
	reWorkdayA = regexp.MustCompile(`([a-zA-Z0-9_-]+)\.(wd\d+)\.myworkdayjobs\.com/(?:en-[A-Z]{2}/)?([a-zA-Z0-9_-]+)/job/`)
	// wd{N}.myworkdaysite.com/[en-US/]recruiting/TENANT/SITE/job/
	reWorkdayB = regexp.MustCompile(`(wd\d+)\.myworkdaysite\.com/(?:en-[A-Z]{2}/)?recruiting/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)/job/`)
)

// DiscoveredSlugs holds slugs parsed from the crowdsourced README sources.
type DiscoveredSlugs struct {
	Greenhouse []companySlug
	Lever      []companySlug
	Workday    []workdayCompany
}

type companySlug struct {
	slug        string
	displayName string
}

// DiscoverSlugs fetches the crowdsourced README files and extracts
// Greenhouse, Lever, and Workday slugs from apply URLs in the tables.
// Falls back to the hardcoded lists if fetching fails.
func DiscoverSlugs(ctx context.Context) DiscoveredSlugs {
	client := &http.Client{Timeout: 20 * time.Second}

	ghSeen := map[string]bool{}
	lvSeen := map[string]bool{}
	wdSeen := map[string]bool{}

	var result DiscoveredSlugs

	for _, url := range readmeSources {
		body, err := fetchText(ctx, client, url)
		if err != nil {
			log.Printf("discover: skipping %s — %v", url, err)
			continue
		}

		// --- Greenhouse ---
		for _, m := range reGreenhouse.FindAllStringSubmatch(body, -1) {
			slug := strings.ToLower(m[1])
			if ghSeen[slug] {
				continue
			}
			ghSeen[slug] = true
			result.Greenhouse = append(result.Greenhouse, companySlug{
				slug:        slug,
				displayName: slug, // display name refined later from API response
			})
		}

		// --- Lever ---
		for _, m := range reLever.FindAllStringSubmatch(body, -1) {
			slug := strings.ToLower(m[1])
			if lvSeen[slug] {
				continue
			}
			lvSeen[slug] = true
			result.Lever = append(result.Lever, companySlug{
				slug:        slug,
				displayName: slug,
			})
		}

		// --- Workday pattern A: tenant.wdN.myworkdayjobs.com/site ---
		for _, m := range reWorkdayA.FindAllStringSubmatch(body, -1) {
			tenant, instance, site := m[1], m[2], m[3]
			key := tenant + "/" + site
			if wdSeen[key] {
				continue
			}
			wdSeen[key] = true
			result.Workday = append(result.Workday, workdayCompany{
				displayName: tenant,
				tenant:      tenant,
				instance:    instance,
				site:        site,
			})
		}

		// --- Workday pattern B: wdN.myworkdaysite.com/recruiting/tenant/site ---
		for _, m := range reWorkdayB.FindAllStringSubmatch(body, -1) {
			instance, tenant, site := m[1], m[2], m[3]
			key := tenant + "/" + site
			if wdSeen[key] {
				continue
			}
			wdSeen[key] = true
			result.Workday = append(result.Workday, workdayCompany{
				displayName: tenant,
				tenant:      tenant,
				instance:    instance,
				site:        site,
				siteBase:    "https://jobs.myworkdaysite.com",
			})
		}
	}

	log.Printf("discover: found %d greenhouse, %d lever, %d workday slugs",
		len(result.Greenhouse), len(result.Lever), len(result.Workday))

	// Fall back to hardcoded lists if discovery found nothing
	// (e.g. network unavailable or GitHub rate-limited).
	if len(result.Greenhouse) == 0 {
		log.Println("discover: greenhouse discovery empty, using fallback list")
		for _, c := range greenhouseCompanies {
			result.Greenhouse = append(result.Greenhouse, companySlug{c.slug, c.displayName})
		}
	}
	if len(result.Lever) == 0 {
		log.Println("discover: lever discovery empty, using fallback list")
		for _, c := range leverCompanies {
			result.Lever = append(result.Lever, companySlug{c.slug, c.displayName})
		}
	}

	return result
}

func fetchText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
