package crawler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sync"

	"internship-tracker/internal/model"
)

// Run discovers slugs dynamically, then fans out crawlers for all three
// ATS sources. Results are deduplicated and written to out.
// out is NOT closed here — caller closes it after Run returns.
func Run(ctx context.Context, out chan<- model.Job) error {
	log.Println("crawler: discovering slugs from crowdsourced lists...")
	slugs := DiscoverSlugs(ctx)

	var dedupMap sync.Map

	ghJobs := make(chan model.Job, 128)
	lvJobs := make(chan model.Job, 128)
	wdJobs := make(chan model.Job, 128)

	g, gCtx := NewGroup(ctx)

	g.Go(func() error {
		defer close(ghJobs)
		return RunGreenhouse(gCtx, ghJobs, slugs.Greenhouse)
	})

	g.Go(func() error {
		defer close(lvJobs)
		return RunLever(gCtx, lvJobs, slugs.Lever)
	})

	g.Go(func() error {
		defer close(wdJobs)
		return RunWorkday(gCtx, wdJobs, slugs.Workday)
	})

	// Fan-in: no gCtx.Done() case — crawlers close channels on cancel,
	// which drives the loop to exit naturally without deadlocking.
	fanInDone := make(chan struct{})
	go func() {
		defer close(fanInDone)
		for ghJobs != nil || lvJobs != nil || wdJobs != nil {
			select {
			case job, ok := <-ghJobs:
				if !ok {
					ghJobs = nil
					continue
				}
				maybeSend(out, job, &dedupMap)
			case job, ok := <-lvJobs:
				if !ok {
					lvJobs = nil
					continue
				}
				maybeSend(out, job, &dedupMap)
			case job, ok := <-wdJobs:
				if !ok {
					wdJobs = nil
					continue
				}
				maybeSend(out, job, &dedupMap)
			}
		}
	}()

	err := g.Wait()
	<-fanInDone
	return err
}

func maybeSend(out chan<- model.Job, job model.Job, dedupMap *sync.Map) {
	key := dedupKey(job)
	if _, loaded := dedupMap.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	out <- job
}

func dedupKey(job model.Job) string {
	h := sha256.Sum256([]byte(job.ID))
	return fmt.Sprintf("%x", h[:8])
}
