package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/apognu/gocal"
)

// CalendarEvent represents a matched calendar event ready to report.
type CalendarEvent struct {
	UID         string
	Title       string
	Start       time.Time
	End         time.Time
	Description string
	Location    string
	Categories  string        // comma-separated
	StartsIn    time.Duration // only meaningful for EventUpcoming
}

// EventFilter holds lowered substring filters for calendar events.
type EventFilter struct {
	Title              string // lowered
	Location           string // lowered
	Description        string // lowered
	Category           string // lowered
	ExcludeTitle       string // lowered
	ExcludeLocation    string // lowered
	ExcludeDescription string // lowered
}

// matchesFilter returns true if the event passes all include/exclude filters.
func matchesFilter(e gocal.Event, f EventFilter) bool {
	// Exclude checks first (fast rejection)
	if f.ExcludeTitle != "" && strings.Contains(strings.ToLower(e.Summary), f.ExcludeTitle) {
		return false
	}
	if f.ExcludeLocation != "" && strings.Contains(strings.ToLower(e.Location), f.ExcludeLocation) {
		return false
	}
	if f.ExcludeDescription != "" && strings.Contains(strings.ToLower(e.Description), f.ExcludeDescription) {
		return false
	}
	// Include checks (AND — all must match)
	if f.Title != "" && !strings.Contains(strings.ToLower(e.Summary), f.Title) {
		return false
	}
	if f.Location != "" && !strings.Contains(strings.ToLower(e.Location), f.Location) {
		return false
	}
	if f.Description != "" && !strings.Contains(strings.ToLower(e.Description), f.Description) {
		return false
	}
	if f.Category != "" {
		found := false
		for _, cat := range e.Categories {
			if strings.Contains(strings.ToLower(cat), f.Category) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Poller polls a calendar source and emits matching events.
type Poller struct {
	source       string
	triggerType  string
	pollInterval time.Duration
	lookAhead    time.Duration // only for EventUpcoming
	filter       EventFilter
	fired        map[string]time.Time
	mu           sync.Mutex
	events       chan CalendarEvent
	done         chan struct{}
}

// StartPoller creates and starts a poll loop. Returns the event channel, a stop
// function, and any setup error.
func StartPoller(source, triggerType string, pollInterval, lookAhead time.Duration, filter EventFilter) (<-chan CalendarEvent, func(), error) {
	if source == "" {
		return nil, nil, fmt.Errorf("source is required")
	}
	if pollInterval <= 0 {
		return nil, nil, fmt.Errorf("poll_interval must be positive, got %s", pollInterval)
	}
	if triggerType == "EventUpcoming" && lookAhead <= 0 {
		return nil, nil, fmt.Errorf("look_ahead must be positive, got %s", lookAhead)
	}

	p := &Poller{
		source:       source,
		triggerType:  triggerType,
		pollInterval: pollInterval,
		lookAhead:    lookAhead,
		filter:       filter,
		fired:        make(map[string]time.Time),
		events:       make(chan CalendarEvent, 16),
		done:         make(chan struct{}),
	}

	go p.loop()

	stop := func() {
		close(p.done)
	}

	return p.events, stop, nil
}

func (p *Poller) loop() {
	defer close(p.events)

	// Poll immediately on start, then on each tick.
	p.poll()

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.poll()
		case <-p.done:
			return
		}
	}
}

func (p *Poller) poll() {
	now := time.Now()

	reader, err := fetchSource(p.source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calendar: fetch source: %v\n", err)
		return
	}
	if closer, ok := reader.(io.Closer); ok {
		defer func() { _ = closer.Close() }()
	}

	matched := p.matchEvents(reader, now)

	p.mu.Lock()
	defer p.mu.Unlock()

	// Prune old dedup entries.
	cutoff := now.Add(-2 * p.pollInterval)
	for key, firedAt := range p.fired {
		if firedAt.Before(cutoff) {
			delete(p.fired, key)
		}
	}

	for _, evt := range matched {
		dedupKey := evt.UID + ":" + evt.Start.Format(time.RFC3339)
		if _, already := p.fired[dedupKey]; already {
			continue
		}
		p.fired[dedupKey] = now

		select {
		case p.events <- evt:
		case <-p.done:
			return
		}
	}
}

// matchEvents parses the iCal data and returns events matching the trigger type.
func (p *Poller) matchEvents(reader io.Reader, now time.Time) []CalendarEvent {
	var windowStart, windowEnd time.Time

	switch p.triggerType {
	case "EventUpcoming":
		windowStart = now
		windowEnd = now.Add(p.lookAhead)
	case "EventStarted":
		windowStart = now.Add(-p.pollInterval)
		windowEnd = now
	case "EventEnded":
		windowStart = now.Add(-p.pollInterval)
		windowEnd = now
	default:
		return nil
	}

	cal := gocal.NewParser(reader)
	cal.Start, cal.End = &windowStart, &windowEnd
	if err := cal.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "calendar: parse error: %v\n", err)
		return nil
	}

	var results []CalendarEvent
	for _, e := range cal.Events {
		if e.Start == nil || e.End == nil {
			continue
		}

		if !matchesFilter(e, p.filter) {
			continue
		}

		categories := strings.Join(e.Categories, ",")

		switch p.triggerType {
		case "EventUpcoming":
			// gocal already filtered to [windowStart, windowEnd] by start time.
			results = append(results, CalendarEvent{
				UID:         e.Uid,
				Title:       e.Summary,
				Start:       *e.Start,
				End:         *e.End,
				Description: e.Description,
				Location:    e.Location,
				Categories:  categories,
				StartsIn:    time.Until(*e.Start),
			})
		case "EventStarted":
			// Event start time must be in [windowStart, windowEnd].
			if !e.Start.Before(windowStart) && !e.Start.After(windowEnd) {
				results = append(results, CalendarEvent{
					UID:         e.Uid,
					Title:       e.Summary,
					Start:       *e.Start,
					End:         *e.End,
					Description: e.Description,
					Location:    e.Location,
					Categories:  categories,
				})
			}
		case "EventEnded":
			// Event end time must be in [windowStart, windowEnd].
			if !e.End.Before(windowStart) && !e.End.After(windowEnd) {
				results = append(results, CalendarEvent{
					UID:         e.Uid,
					Title:       e.Summary,
					Start:       *e.Start,
					End:         *e.End,
					Description: e.Description,
					Location:    e.Location,
					Categories:  categories,
				})
			}
		}
	}

	return results
}

// fetchSource returns a reader for the calendar data. HTTP(S) URLs are fetched
// with a 30-second timeout; anything else is treated as a local file path.
func fetchSource(source string) (io.Reader, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return nil, fmt.Errorf("HTTP GET %s: %w", source, err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("HTTP GET %s: status %d", source, resp.StatusCode)
		}
		return resp.Body, nil
	}

	f, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", source, err)
	}
	return f, nil
}

// IsURL returns true if the source string looks like an HTTP(S) URL.
func IsURL(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}
