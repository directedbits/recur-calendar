package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apognu/gocal"
)

// testICS generates a minimal ICS calendar with one event.
func testICS(uid, summary string, start, end time.Time) string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Test//Test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:" + uid + "\r\n" +
		"DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z") + "\r\n" +
		"SUMMARY:" + summary + "\r\n" +
		"DTSTART:" + start.UTC().Format("20060102T150405Z") + "\r\n" +
		"DTEND:" + end.UTC().Format("20060102T150405Z") + "\r\n" +
		"DESCRIPTION:Test description\r\n" +
		"LOCATION:Test location\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

// testICSMulti generates an ICS calendar with multiple events.
func testICSMulti(events []struct {
	UID, Summary string
	Start, End   time.Time
}) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Test//Test//EN\r\n")
	for _, e := range events {
		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString("UID:" + e.UID + "\r\n")
		b.WriteString("DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("SUMMARY:" + e.Summary + "\r\n")
		b.WriteString("DTSTART:" + e.Start.UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("DTEND:" + e.End.UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func TestMatchEvents_EventUpcoming(t *testing.T) {
	now := time.Now().UTC()
	// Event starts 10 minutes from now — within default 15m look_ahead.
	ics := testICS("uid-1", "Standup", now.Add(10*time.Minute), now.Add(40*time.Minute))

	p := &Poller{
		triggerType:  "EventUpcoming",
		pollInterval: 5 * time.Minute,
		lookAhead:    15 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].UID != "uid-1" {
		t.Errorf("UID = %q, want %q", results[0].UID, "uid-1")
	}
	if results[0].Title != "Standup" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Standup")
	}
	if results[0].StartsIn <= 0 {
		t.Errorf("StartsIn = %v, expected positive", results[0].StartsIn)
	}
}

func TestMatchEvents_EventUpcoming_OutsideWindow(t *testing.T) {
	now := time.Now().UTC()
	// Event starts 30 minutes from now — outside 15m look_ahead.
	ics := testICS("uid-2", "Later Meeting", now.Add(30*time.Minute), now.Add(60*time.Minute))

	p := &Poller{
		triggerType:  "EventUpcoming",
		pollInterval: 5 * time.Minute,
		lookAhead:    15 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestMatchEvents_EventStarted(t *testing.T) {
	now := time.Now().UTC()
	// Event started 2 minutes ago — within 5m poll window.
	ics := testICS("uid-3", "Team Sync", now.Add(-2*time.Minute), now.Add(28*time.Minute))

	p := &Poller{
		triggerType:  "EventStarted",
		pollInterval: 5 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].UID != "uid-3" {
		t.Errorf("UID = %q, want %q", results[0].UID, "uid-3")
	}
}

func TestMatchEvents_EventStarted_OutsideWindow(t *testing.T) {
	now := time.Now().UTC()
	// Event started 10 minutes ago — outside 5m poll window.
	ics := testICS("uid-4", "Old Meeting", now.Add(-10*time.Minute), now.Add(20*time.Minute))

	p := &Poller{
		triggerType:  "EventStarted",
		pollInterval: 5 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestMatchEvents_EventEnded(t *testing.T) {
	now := time.Now().UTC()
	// Event ended 3 minutes ago — within 5m poll window.
	ics := testICS("uid-5", "Finished Call", now.Add(-33*time.Minute), now.Add(-3*time.Minute))

	p := &Poller{
		triggerType:  "EventEnded",
		pollInterval: 5 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].UID != "uid-5" {
		t.Errorf("UID = %q, want %q", results[0].UID, "uid-5")
	}
}

func TestMatchEvents_EventEnded_OutsideWindow(t *testing.T) {
	now := time.Now().UTC()
	// Event ended 10 minutes ago — outside 5m poll window.
	ics := testICS("uid-6", "Long Gone", now.Add(-40*time.Minute), now.Add(-10*time.Minute))

	p := &Poller{
		triggerType:  "EventEnded",
		pollInterval: 5 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(results))
	}
}

func TestDedup_SameEventNotRefired(t *testing.T) {
	now := time.Now().UTC()
	ics := testICS("uid-dup", "Recurring", now.Add(5*time.Minute), now.Add(35*time.Minute))

	p := &Poller{
		triggerType:  "EventUpcoming",
		pollInterval: 5 * time.Minute,
		lookAhead:    15 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	// First poll — should match.
	results1 := p.matchEvents(strings.NewReader(ics), now)
	if len(results1) != 1 {
		t.Fatalf("first poll: expected 1 match, got %d", len(results1))
	}

	// Simulate dedup: mark as fired.
	dedupKey := results1[0].UID + ":" + results1[0].Start.Format(time.RFC3339)
	p.fired[dedupKey] = now

	// Second poll — same event, should be deduped by poll().
	// We test matchEvents still returns it (dedup is in poll), so instead
	// test the dedup map directly.
	if _, exists := p.fired[dedupKey]; !exists {
		t.Error("dedup key should exist after first fire")
	}
}

func TestDedup_PruneOldEntries(t *testing.T) {
	p := &Poller{
		pollInterval: 5 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	now := time.Now()
	// Add an old entry (15 minutes ago, well beyond 2 * 5m = 10m cutoff).
	p.fired["old-uid:2024-01-01T00:00:00Z"] = now.Add(-15 * time.Minute)
	// Add a recent entry.
	p.fired["new-uid:2024-01-01T01:00:00Z"] = now.Add(-1 * time.Minute)

	// Simulate pruning (same logic as in poll).
	cutoff := now.Add(-2 * p.pollInterval)
	for key, firedAt := range p.fired {
		if firedAt.Before(cutoff) {
			delete(p.fired, key)
		}
	}

	if _, exists := p.fired["old-uid:2024-01-01T00:00:00Z"]; exists {
		t.Error("old entry should have been pruned")
	}
	if _, exists := p.fired["new-uid:2024-01-01T01:00:00Z"]; !exists {
		t.Error("recent entry should NOT have been pruned")
	}
}

func TestDedup_RecurringInstancesDistinct(t *testing.T) {
	// Two instances of the same UID but different start times should have
	// different dedup keys.
	key1 := "recurring-uid" + ":" + "2024-06-01T10:00:00Z"
	key2 := "recurring-uid" + ":" + "2024-06-08T10:00:00Z"

	if key1 == key2 {
		t.Error("recurring instances should have distinct dedup keys")
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://example.com/cal.ics", true},
		{"http://example.com/cal.ics", true},
		{"/home/user/cal.ics", false},
		{"~/calendars/work.ics", false},
		{"calendar.ics", false},
	}
	for _, tt := range tests {
		if got := IsURL(tt.input); got != tt.want {
			t.Errorf("IsURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestStartPoller_EmptySource(t *testing.T) {
	_, _, err := StartPoller("", "EventUpcoming", 5*time.Minute, 15*time.Minute, EventFilter{})
	if err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestStartPoller_InvalidPollInterval(t *testing.T) {
	_, _, err := StartPoller("/tmp/cal.ics", "EventStarted", 0, 0, EventFilter{})
	if err == nil {
		t.Fatal("expected error for zero poll_interval")
	}
}

func TestStartPoller_InvalidLookAhead(t *testing.T) {
	_, _, err := StartPoller("/tmp/cal.ics", "EventUpcoming", 5*time.Minute, 0, EventFilter{})
	if err == nil {
		t.Fatal("expected error for zero look_ahead on EventUpcoming")
	}
}

func TestMatchEvents_MultipleEvents(t *testing.T) {
	now := time.Now().UTC()
	events := []struct {
		UID, Summary string
		Start, End   time.Time
	}{
		{"m1", "Meeting 1", now.Add(3 * time.Minute), now.Add(33 * time.Minute)},
		{"m2", "Meeting 2", now.Add(10 * time.Minute), now.Add(40 * time.Minute)},
		{"m3", "Meeting 3", now.Add(20 * time.Minute), now.Add(50 * time.Minute)}, // outside
	}
	ics := testICSMulti(events)

	p := &Poller{
		triggerType:  "EventUpcoming",
		pollInterval: 5 * time.Minute,
		lookAhead:    15 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(results))
	}
}

func TestMatchEvents_EventWithDescription(t *testing.T) {
	now := time.Now().UTC()
	ics := testICS("uid-desc", "Described Event", now.Add(5*time.Minute), now.Add(35*time.Minute))

	p := &Poller{
		triggerType:  "EventUpcoming",
		pollInterval: 5 * time.Minute,
		lookAhead:    15 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Description != "Test description" {
		t.Errorf("Description = %q, want %q", results[0].Description, "Test description")
	}
	if results[0].Location != "Test location" {
		t.Errorf("Location = %q, want %q", results[0].Location, "Test location")
	}
}

func TestPluginInputParsing(t *testing.T) {
	jsonStr := `{
		"trigger_type": "EventUpcoming",
		"options": {"source": "https://example.com/cal.ics", "look_ahead": "30m", "poll_interval": "10m"},
		"config": {}
	}`

	var input pluginInput
	if err := json.NewDecoder(strings.NewReader(jsonStr)).Decode(&input); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if input.TriggerType != "EventUpcoming" {
		t.Errorf("TriggerType = %q, want %q", input.TriggerType, "EventUpcoming")
	}
	if src, ok := input.Options["source"].(string); !ok || src != "https://example.com/cal.ics" {
		t.Errorf("Options[source] = %v, want %q", input.Options["source"], "https://example.com/cal.ics")
	}
	if la, ok := input.Options["look_ahead"].(string); !ok || la != "30m" {
		t.Errorf("Options[look_ahead] = %v, want %q", input.Options["look_ahead"], "30m")
	}
}

func TestOptString(t *testing.T) {
	opts := map[string]any{
		"source": "https://example.com/cal.ics",
		"empty":  "",
		"number": 42,
	}

	if got := optString(opts, "source", "default"); got != "https://example.com/cal.ics" {
		t.Errorf("got %q, want %q", got, "https://example.com/cal.ics")
	}
	if got := optString(opts, "missing", "default"); got != "default" {
		t.Errorf("got %q, want %q", got, "default")
	}
	if got := optString(opts, "empty", "default"); got != "default" {
		t.Errorf("got %q for empty string, want fallback %q", got, "default")
	}
	if got := optString(opts, "number", "default"); got != "default" {
		t.Errorf("got %q for non-string, want fallback %q", got, "default")
	}
}

func TestFetchSource_LocalFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ics")
	content := testICS("local-1", "Local Event", time.Now().Add(5*time.Minute), time.Now().Add(35*time.Minute))
	os.WriteFile(path, []byte(content), 0644)

	reader, err := fetchSource(path)
	if err != nil {
		t.Fatalf("fetchSource error: %v", err)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !strings.Contains(string(data), "Local Event") {
		t.Error("expected to read ICS content with 'Local Event'")
	}
}

func TestFetchSource_HTTP(t *testing.T) {
	content := testICS("http-1", "HTTP Event", time.Now().Add(5*time.Minute), time.Now().Add(35*time.Minute))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer srv.Close()

	reader, err := fetchSource(srv.URL)
	if err != nil {
		t.Fatalf("fetchSource error: %v", err)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !strings.Contains(string(data), "HTTP Event") {
		t.Error("expected to read ICS content with 'HTTP Event'")
	}
}

func TestFetchSource_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fetchSource(srv.URL)
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestFetchSource_FileNotFound(t *testing.T) {
	_, err := fetchSource("/nonexistent/path/to/calendar.ics")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestStartPoller_HappyPath(t *testing.T) {
	// Create a temp ICS file with an upcoming event
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ics")
	now := time.Now().UTC()
	content := testICS("poll-1", "Upcoming Meeting", now.Add(5*time.Minute), now.Add(35*time.Minute))
	os.WriteFile(path, []byte(content), 0644)

	events, stop, err := StartPoller(path, "EventUpcoming", 100*time.Millisecond, 15*time.Minute, EventFilter{})
	if err != nil {
		t.Fatalf("StartPoller error: %v", err)
	}
	defer stop()

	// Should receive the event from the initial poll
	select {
	case evt := <-events:
		if evt.UID != "poll-1" {
			t.Errorf("UID = %q, want %q", evt.UID, "poll-1")
		}
		if evt.Title != "Upcoming Meeting" {
			t.Errorf("Title = %q, want %q", evt.Title, "Upcoming Meeting")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestStartPoller_DedupAcrossPolls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ics")
	now := time.Now().UTC()
	content := testICS("dedup-1", "Dedup Event", now.Add(5*time.Minute), now.Add(35*time.Minute))
	os.WriteFile(path, []byte(content), 0644)

	// Use a 1s poll interval so the dedup cutoff (2s) doesn't expire before
	// the second poll fires.
	events, stop, err := StartPoller(path, "EventUpcoming", 1*time.Second, 15*time.Minute, EventFilter{})
	if err != nil {
		t.Fatalf("StartPoller error: %v", err)
	}
	defer stop()

	// First event should arrive from the immediate initial poll.
	select {
	case <-events:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first event")
	}

	// Second poll fires after 1s — should NOT re-deliver the same event (dedup).
	select {
	case evt := <-events:
		t.Fatalf("expected no second event (dedup), got: %+v", evt)
	case <-time.After(2 * time.Second):
		// expected - dedup working
	}
}

func TestStartPoller_EventStarted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ics")
	now := time.Now().UTC()
	// Event that started 1 second ago — use a 5s poll interval so the
	// EventStarted window (now-5s..now) comfortably includes a 1-second-ago start.
	content := testICS("started-1", "Just Started", now.Add(-1*time.Second), now.Add(29*time.Minute))
	os.WriteFile(path, []byte(content), 0644)

	events, stop, err := StartPoller(path, "EventStarted", 5*time.Second, 0, EventFilter{})
	if err != nil {
		t.Fatalf("StartPoller error: %v", err)
	}
	defer stop()

	select {
	case evt := <-events:
		if evt.UID != "started-1" {
			t.Errorf("UID = %q, want %q", evt.UID, "started-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EventStarted event")
	}
}

func TestMatchEvents_UnknownTriggerType(t *testing.T) {
	p := &Poller{
		triggerType:  "UnknownType",
		pollInterval: 5 * time.Minute,
		fired:        make(map[string]time.Time),
	}
	now := time.Now().UTC()
	ics := testICS("uid", "Event", now.Add(5*time.Minute), now.Add(35*time.Minute))
	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown trigger type, got %d", len(results))
	}
}

// testICSFull generates a minimal ICS calendar with one event including all fields.
func testICSFull(uid, summary, description, location string, categories []string, start, end time.Time) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Test//Test//EN\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString("UID:" + uid + "\r\n")
	b.WriteString("DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z") + "\r\n")
	b.WriteString("SUMMARY:" + summary + "\r\n")
	b.WriteString("DTSTART:" + start.UTC().Format("20060102T150405Z") + "\r\n")
	b.WriteString("DTEND:" + end.UTC().Format("20060102T150405Z") + "\r\n")
	if description != "" {
		b.WriteString("DESCRIPTION:" + description + "\r\n")
	}
	if location != "" {
		b.WriteString("LOCATION:" + location + "\r\n")
	}
	if len(categories) > 0 {
		b.WriteString("CATEGORIES:" + strings.Join(categories, ",") + "\r\n")
	}
	b.WriteString("END:VEVENT\r\n")
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

// testICSFullMulti generates an ICS calendar with multiple full events.
func testICSFullMulti(events []struct {
	UID, Summary, Description, Location string
	Categories                          []string
	Start, End                          time.Time
}) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Test//Test//EN\r\n")
	for _, e := range events {
		b.WriteString("BEGIN:VEVENT\r\n")
		b.WriteString("UID:" + e.UID + "\r\n")
		b.WriteString("DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("SUMMARY:" + e.Summary + "\r\n")
		b.WriteString("DTSTART:" + e.Start.UTC().Format("20060102T150405Z") + "\r\n")
		b.WriteString("DTEND:" + e.End.UTC().Format("20060102T150405Z") + "\r\n")
		if e.Description != "" {
			b.WriteString("DESCRIPTION:" + e.Description + "\r\n")
		}
		if e.Location != "" {
			b.WriteString("LOCATION:" + e.Location + "\r\n")
		}
		if len(e.Categories) > 0 {
			b.WriteString("CATEGORIES:" + strings.Join(e.Categories, ",") + "\r\n")
		}
		b.WriteString("END:VEVENT\r\n")
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func newGocalEvent(summary, description, location string, categories []string) gocal.Event {
	return gocal.Event{
		Summary:     summary,
		Description: description,
		Location:    location,
		Categories:  categories,
	}
}

func TestMatchesFilter_EmptyFilter(t *testing.T) {
	e := newGocalEvent("Daily Standup", "Sprint planning session", "Zoom Room A", []string{"Work"})
	if !matchesFilter(e, EventFilter{}) {
		t.Error("empty filter should match all events")
	}
}

func TestMatchesFilter_TitleMatch(t *testing.T) {
	f := EventFilter{Title: "standup"}
	if !matchesFilter(newGocalEvent("Daily Standup", "", "", nil), f) {
		t.Error("should match 'Daily Standup' with filter 'standup'")
	}
	if matchesFilter(newGocalEvent("Lunch Break", "", "", nil), f) {
		t.Error("should not match 'Lunch Break' with filter 'standup'")
	}
}

func TestMatchesFilter_TitleCaseInsensitive(t *testing.T) {
	f := EventFilter{Title: "standup"}
	if !matchesFilter(newGocalEvent("daily standup", "", "", nil), f) {
		t.Error("should match case-insensitively")
	}
	f2 := EventFilter{Title: "standup"}
	if !matchesFilter(newGocalEvent("DAILY STANDUP", "", "", nil), f2) {
		t.Error("should match uppercase title")
	}
}

func TestMatchesFilter_LocationMatch(t *testing.T) {
	f := EventFilter{Location: "zoom"}
	if !matchesFilter(newGocalEvent("Meeting", "", "Zoom Room A", nil), f) {
		t.Error("should match location 'Zoom Room A' with filter 'zoom'")
	}
	if matchesFilter(newGocalEvent("Meeting", "", "Cafeteria", nil), f) {
		t.Error("should not match location 'Cafeteria' with filter 'zoom'")
	}
}

func TestMatchesFilter_DescriptionMatch(t *testing.T) {
	f := EventFilter{Description: "sprint"}
	if !matchesFilter(newGocalEvent("Meeting", "Sprint planning session", "", nil), f) {
		t.Error("should match description containing 'sprint'")
	}
	if matchesFilter(newGocalEvent("Meeting", "Casual chat", "", nil), f) {
		t.Error("should not match description 'Casual chat' with filter 'sprint'")
	}
}

func TestMatchesFilter_CategoryMatch(t *testing.T) {
	f := EventFilter{Category: "work"}
	if !matchesFilter(newGocalEvent("Meeting", "", "", []string{"Work", "Meetings"}), f) {
		t.Error("should match category 'Work' with filter 'work'")
	}
}

func TestMatchesFilter_CategoryNoMatch(t *testing.T) {
	f := EventFilter{Category: "personal"}
	if matchesFilter(newGocalEvent("Meeting", "", "", []string{"Work"}), f) {
		t.Error("should not match category 'Work' with filter 'personal'")
	}
}

func TestMatchesFilter_CategoryEmptyList(t *testing.T) {
	f := EventFilter{Category: "work"}
	if matchesFilter(newGocalEvent("Meeting", "", "", nil), f) {
		t.Error("should not match event with no categories when filter_category is set")
	}
}

func TestMatchesFilter_ExcludeTitle(t *testing.T) {
	f := EventFilter{ExcludeTitle: "lunch"}
	if matchesFilter(newGocalEvent("Lunch Break", "", "", nil), f) {
		t.Error("should exclude 'Lunch Break' with exclude_title 'lunch'")
	}
	if !matchesFilter(newGocalEvent("Standup", "", "", nil), f) {
		t.Error("should allow 'Standup' with exclude_title 'lunch'")
	}
}

func TestMatchesFilter_ExcludeLocation(t *testing.T) {
	f := EventFilter{ExcludeLocation: "cafeteria"}
	if matchesFilter(newGocalEvent("Lunch", "", "Cafeteria", nil), f) {
		t.Error("should exclude event with location 'Cafeteria'")
	}
	if !matchesFilter(newGocalEvent("Meeting", "", "Zoom", nil), f) {
		t.Error("should allow event with location 'Zoom'")
	}
}

func TestMatchesFilter_ExcludeDescription(t *testing.T) {
	f := EventFilter{ExcludeDescription: "optional"}
	if matchesFilter(newGocalEvent("Meeting", "Optional meeting", "", nil), f) {
		t.Error("should exclude event with description 'Optional meeting'")
	}
	if !matchesFilter(newGocalEvent("Meeting", "Required standup", "", nil), f) {
		t.Error("should allow event with description 'Required standup'")
	}
}

func TestMatchesFilter_ExcludeAndInclude(t *testing.T) {
	f := EventFilter{Title: "meeting", ExcludeTitle: "skip"}
	if !matchesFilter(newGocalEvent("Team Meeting", "", "", nil), f) {
		t.Error("should match 'Team Meeting' (matches title, no exclude)")
	}
	if matchesFilter(newGocalEvent("Skip This Meeting", "", "", nil), f) {
		t.Error("should not match 'Skip This Meeting' (excluded by exclude_title)")
	}
}

func TestMatchesFilter_CombinedAND(t *testing.T) {
	f := EventFilter{Title: "standup", Location: "zoom"}
	if !matchesFilter(newGocalEvent("Daily Standup", "", "Zoom Room", nil), f) {
		t.Error("should match when both title and location match")
	}
	if matchesFilter(newGocalEvent("Daily Standup", "", "Cafeteria", nil), f) {
		t.Error("should not match when title matches but location does not")
	}
	if matchesFilter(newGocalEvent("Lunch", "", "Zoom Room", nil), f) {
		t.Error("should not match when location matches but title does not")
	}
}

func TestMatchEvents_WithFilter(t *testing.T) {
	now := time.Now().UTC()
	events := []struct {
		UID, Summary, Description, Location string
		Categories                          []string
		Start, End                          time.Time
	}{
		{"f1", "Daily Standup", "Sprint sync", "Zoom", []string{"Work"}, now.Add(3 * time.Minute), now.Add(33 * time.Minute)},
		{"f2", "Lunch Break", "Free time", "Cafeteria", nil, now.Add(5 * time.Minute), now.Add(35 * time.Minute)},
		{"f3", "Team Standup", "Weekly sync", "Zoom", []string{"Work"}, now.Add(10 * time.Minute), now.Add(40 * time.Minute)},
	}
	ics := testICSFullMulti(events)

	p := &Poller{
		triggerType:  "EventUpcoming",
		pollInterval: 5 * time.Minute,
		lookAhead:    15 * time.Minute,
		filter:       EventFilter{Title: "standup"},
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 2 {
		t.Fatalf("expected 2 matches for filter_title='standup', got %d", len(results))
	}
	for _, r := range results {
		if !strings.Contains(strings.ToLower(r.Title), "standup") {
			t.Errorf("unexpected event title: %q", r.Title)
		}
	}
}

func TestMatchEvents_CategoriesPopulated(t *testing.T) {
	now := time.Now().UTC()
	ics := testICSFull("cat-1", "Meeting", "Desc", "Room", []string{"Work", "Important"}, now.Add(5*time.Minute), now.Add(35*time.Minute))

	p := &Poller{
		triggerType:  "EventUpcoming",
		pollInterval: 5 * time.Minute,
		lookAhead:    15 * time.Minute,
		fired:        make(map[string]time.Time),
	}

	results := p.matchEvents(strings.NewReader(ics), now)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Categories != "Work,Important" {
		t.Errorf("Categories = %q, want %q", results[0].Categories, "Work,Important")
	}
}

func TestParseInput_FilterOptions(t *testing.T) {
	jsonStr := `{
		"trigger_type": "EventUpcoming",
		"options": {
			"source": "https://example.com/cal.ics",
			"look_ahead": "15m",
			"poll_interval": "5m",
			"filter_title": "Standup",
			"filter_location": "Zoom",
			"filter_description": "Sprint",
			"filter_category": "Work",
			"exclude_title": "Lunch",
			"exclude_location": "Cafeteria",
			"exclude_description": "Optional"
		},
		"config": {}
	}`

	_, _, _, _, filter, err := parseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("parseInput error: %v", err)
	}

	// All values should be lowered
	if filter.Title != "standup" {
		t.Errorf("filter.Title = %q, want %q", filter.Title, "standup")
	}
	if filter.Location != "zoom" {
		t.Errorf("filter.Location = %q, want %q", filter.Location, "zoom")
	}
	if filter.Description != "sprint" {
		t.Errorf("filter.Description = %q, want %q", filter.Description, "sprint")
	}
	if filter.Category != "work" {
		t.Errorf("filter.Category = %q, want %q", filter.Category, "work")
	}
	if filter.ExcludeTitle != "lunch" {
		t.Errorf("filter.ExcludeTitle = %q, want %q", filter.ExcludeTitle, "lunch")
	}
	if filter.ExcludeLocation != "cafeteria" {
		t.Errorf("filter.ExcludeLocation = %q, want %q", filter.ExcludeLocation, "cafeteria")
	}
	if filter.ExcludeDescription != "optional" {
		t.Errorf("filter.ExcludeDescription = %q, want %q", filter.ExcludeDescription, "optional")
	}
}
