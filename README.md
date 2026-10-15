# Intern Canada

**The next version of the student job hunt.** A live feed of Canadian internship and co-op postings that shows what is open right now, remembers where you stand, and tells you when to follow up. No account needed.

Built for **ThunderHacks 2026** (Main Track: *Version 2.0*).

## Why this matters

Getting a first foothold is hard right now. Statistics Canada reported youth (15 to 24) unemployment of [12.7% in June 2026](https://www150.statcan.gc.ca/daily-quotidien/260710/dq260710a-eng.htm), and it peaked at 14.6% in September 2025, the [highest in about 15 years outside the pandemic](https://www.cp24.com/news/canada/2026/04/22/youth-unemployment-on-the-rise-amid-worsening-labour-conditions-report/), according to a Statistics Canada-based report. Internships and co-ops are a main bridge from a degree to a first job.

We can't create jobs. What we can do is stop the search itself from wasting the effort students already spend.

## Version 1.0: the hunt, as it works today

A student's routine looks like this: ten career pages open in tabs, a hand-kept spreadsheet, and a big shared list of apply links that other students copy from (for example [Canadian-Tech-Internships-2026](https://github.com/negarprh/Canadian-Tech-Internships-2026)).

**What is broken**

- **Dead links.** Postings close but stay in the list, so you find out after you click.
  [TODO: audit result, e.g. "X of Y checkable links in the 2026 list were already closed on Oct 15"]
- **Postings vanish overnight.** [TODO: "N postings closed between yesterday's crawl and this morning's"]
- **No search or filters.** You scroll hundreds of rows to find co-ops in your province.
- **Nothing tells you what's new.**
- **The tracking lives somewhere else.** Saved, applied and interview status sits in a spreadsheet, and nothing reminds you to follow up.

## Version 2.0: what this does

- **Live feed.** Postings stream into the browser over WebSockets as the crawler finds them.
- **Only what's open.** Postings that disappear from the company board are marked closed and hidden by default.
- **Search and filters.** Text, co-op or internship, province or remote, your status, and "new in the last 24 hours".
- **Built-in tracker.** Mark any posting Saved, Applied, Interview or Offer. Each status is dated, so you see "Applied Oct 12".
- **Follow-up nudge.** Applications older than a week are flagged so nothing gets forgotten.
- **Closed alerts.** If a posting you saved or applied to closes, a banner tells you.
- **Back up or move your tracker.** Export it as a JSON file and import it on another device. Your statuses live in your browser, so there is no account and no sign-up.
- **Accessible.** Keyboard operable, labelled controls, visible focus, WCAG AA text contrast, a skip link, and reduced-motion support.
- **Works on a phone.** Rows become cards on small screens.

## Before and after

| | Version 1.0 (list + spreadsheet + tabs) | Version 2.0 (Intern Canada) |
|---|---|---|
| Find 3 relevant open roles | [TODO: measured time] | [TODO: measured time] |
| Dead links you click | [TODO: count] | 0 shown by default |
| See what is new today | Not possible | One toggle |
| Track application status | Separate spreadsheet | One dropdown per row, dated |
| Reminded to follow up | No | Flagged after 7 days |
| Learn it | Read the repo | Open the page |

[TODO: add 3 to 5 short quotes from students who tried both versions]

## The 20-minute daily routine

The software only helps if it fits a habit. This is the process we suggest:

1. **Check the feed** with "New in 24h" on (5 min).
2. **Save** the postings worth applying to.
3. **Apply to up to three**, then mark each one Applied (10 min).
4. **Clear the follow-ups** the banner lists. Send a short message to anyone you applied to a week ago (5 min).

## Who it includes

Most postings cluster in a few big cities. Filtering to **Remote** helps students who can't relocate for a placement.
[TODO: add the share of postings by city, and a Northern Ontario count, once you have run the crawler. Only keep this section if the numbers support it.]

## Run it

Requires Go 1.22+ and `gcc` (the SQLite driver uses CGo).

```sh
git clone https://github.com/algebrain35/internship-tracker
cd internship-tracker
make run
```

Then open <http://localhost:8080>. The crawler runs in the background and postings appear as they are found.

To keep the data fresh while the server is running:

```sh
./bin/crawler -every 15m
```

To measure how stale the shared lists are (the Version 1.0 evidence):

```sh
go run ./cmd/audit
```

## How it works

```
cmd/crawler → internal/crawler → internal/store (SQLite, WAL mode)
              Greenhouse | Lever | Workday           ↑
              fan-in → dedupe → single DB writer     │
                                                     │
cmd/server  → internal/hub  ←  poller (rowid cursor + closed notices)
                   ↓
            WebSocket clients (web/index.html)
```

- **Discovery.** Company boards are found by parsing the apply URLs in two crowdsourced GitHub lists, with a hardcoded fallback if GitHub is unreachable. Postings then come from the Greenhouse, Lever and Workday endpoints.
  [TODO: confirm Lever and Workday return rows before demo day]
- **Concurrency.** Each source crawls in its own goroutine. Results fan in through channels, are deduplicated with a `sync.Map`, and are written by a single DB writer goroutine. A small fail-fast errgroup cancels the other crawls if one source fails.
- **Closed detection.** Every crawl refreshes `last_seen_at` on each posting it sees. After a complete, uninterrupted crawl, postings that were not seen are marked closed. A failed or interrupted crawl never closes anything.
- **Live delivery.** The crawler and server are separate processes sharing one SQLite file in WAL mode, so the crawler can write while the server reads. The server polls with a monotonic rowid cursor and pushes new postings and close notices to every client. WebSockets use `net/http` connection hijacking, with no extra dependency.
- **First seen.** A posting's first-seen time is its `fetched_at`, which is never overwritten. This drives "new in 24h".
- **The tracker.** Statuses are stored in the browser's `localStorage` as `{ status, date }` per posting, with no backend and no account.

## Switching from the old way

There is nothing to migrate. Intern Canada reads the same public sources the lists are built from, so your existing list keeps working alongside it. Import your statuses in seconds by marking postings as you go, and take your tracker with you by exporting it.

**Proposed pilot:** share it with the campus career office and ALCOMS, and gather feedback from students during one hiring season.

## Limitations

- **Statuses live in one browser.** They survive reloads and restarts, but not clearing site data, private windows, or a different device. Use Export to back up or move them.
- **SQLite on one machine.** Fine for a hackathon-sized dataset, not for many crawlers.
- **Company names** come from board identifiers, so some show as slugs.
- **Posting dates** are not exposed by every source, so "new" means first seen by this tracker.
- **We don't create jobs.** This reduces wasted effort in the search. It does not change how many positions exist.

## Credits

Built during ThunderHacks 2026 by Team Alpha.