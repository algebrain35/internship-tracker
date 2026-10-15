package store_test

import (
	"os"
	"testing"
	"time"

	"internship-tracker/internal/model"
	"internship-tracker/internal/store"
)

func TestInsertAndPoll(t *testing.T) {
	// Use a temp file so each test run starts clean.
	f, err := os.CreateTemp("", "jobs-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	s, err := store.Open(f.Name())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// --- Insert two jobs ---
	jobs := []model.Job{
		{
			ID:            "greenhouse:shopify:1001",
			Source:        model.SourceGreenhouse,
			FetchedAt:     time.Now().UTC(),
			Title:         "Software Engineering Intern",
			Company:       "Shopify",
			ApplyURL:      "https://boards.greenhouse.io/shopify/jobs/1001",
			LocationRaw:   "Toronto, ON",
			WorkplaceType: model.WorkplaceHybrid,
			Department:    "Engineering",
			Commitment:    "Internship",
			Description:   "<p>Build commerce.</p>",
			PostedAt:      time.Now().Add(-24 * time.Hour).UTC(),
		},
		{
			ID:            "lever:1password:abc-123",
			Source:        model.SourceLever,
			FetchedAt:     time.Now().UTC(),
			Title:         "Security Co-op",
			Company:       "1Password",
			ApplyURL:      "https://jobs.lever.co/1password/abc-123",
			LocationRaw:   "Remote, Canada",
			WorkplaceType: model.WorkplaceRemote,
			Department:    "Security",
			Commitment:    "Co-op",
			Description:   "<p>Keep secrets safe.</p>",
			// PostedAt intentionally zero — Workday-style missing date
		},
	}

	for _, job := range jobs {
		if err := s.Insert(job); err != nil {
			t.Fatalf("Insert(%s): %v", job.ID, err)
		}
	}

	// --- Idempotency: inserting same ID again should be a no-op ---
	if err := s.Insert(jobs[0]); err != nil {
		t.Fatalf("duplicate Insert should not error: %v", err)
	}

	count, err := s.Count()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("want 2 jobs, got %d", count)
	}

	// --- PollSince(0) should return both jobs ---
	fetched, maxRowID, err := s.PollSince(0)
	if err != nil {
		t.Fatalf("PollSince: %v", err)
	}
	if len(fetched) != 2 {
		t.Fatalf("want 2, got %d", len(fetched))
	}
	if maxRowID != 2 {
		t.Fatalf("want maxRowID=2, got %d", maxRowID)
	}

	// --- PollSince(maxRowID) should return nothing new ---
	fetched2, maxRowID2, err := s.PollSince(maxRowID)
	if err != nil {
		t.Fatalf("PollSince second call: %v", err)
	}
	if len(fetched2) != 0 {
		t.Fatalf("want 0 new jobs, got %d", len(fetched2))
	}
	if maxRowID2 != maxRowID {
		t.Fatalf("maxRowID should be unchanged, got %d", maxRowID2)
	}

	t.Logf("✅ Insert, dedup, PollSince all pass. maxRowID=%d", maxRowID)
}
