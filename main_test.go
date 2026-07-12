package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseInput_Valid(t *testing.T) {
	jsonStr := `{"trigger_type":"EventUpcoming","options":{"source":"https://example.com/cal.ics","poll_interval":"10m","look_ahead":"30m"},"config":{}}`
	input, source, pollInterval, lookAhead, _, err := parseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.TriggerType != "EventUpcoming" {
		t.Errorf("TriggerType = %q", input.TriggerType)
	}
	if source != "https://example.com/cal.ics" {
		t.Errorf("source = %q", source)
	}
	if pollInterval != 10*time.Minute {
		t.Errorf("pollInterval = %v", pollInterval)
	}
	if lookAhead != 30*time.Minute {
		t.Errorf("lookAhead = %v", lookAhead)
	}
}

func TestParseInput_EventStarted(t *testing.T) {
	jsonStr := `{"trigger_type":"EventStarted","options":{"source":"test.ics"},"config":{}}`
	input, source, pollInterval, lookAhead, _, err := parseInput(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.TriggerType != "EventStarted" {
		t.Errorf("TriggerType = %q", input.TriggerType)
	}
	if source != "test.ics" {
		t.Errorf("source = %q", source)
	}
	if pollInterval != 5*time.Minute {
		t.Errorf("pollInterval = %v, want 5m (default)", pollInterval)
	}
	if lookAhead != 0 {
		t.Errorf("lookAhead = %v, want 0 (not EventUpcoming)", lookAhead)
	}
}

func TestParseInput_MissingSource(t *testing.T) {
	jsonStr := `{"trigger_type":"EventUpcoming","options":{"poll_interval":"10m","look_ahead":"30m"},"config":{}}`
	_, _, _, _, _, err := parseInput(strings.NewReader(jsonStr))
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestParseInput_InvalidTriggerType(t *testing.T) {
	jsonStr := `{"trigger_type":"BadType","options":{"source":"test.ics"},"config":{}}`
	_, _, _, _, _, err := parseInput(strings.NewReader(jsonStr))
	if err == nil {
		t.Fatal("expected error for invalid trigger_type")
	}
}

func TestParseInput_InvalidPollInterval(t *testing.T) {
	jsonStr := `{"trigger_type":"EventStarted","options":{"source":"test.ics","poll_interval":"bad"},"config":{}}`
	_, _, _, _, _, err := parseInput(strings.NewReader(jsonStr))
	if err == nil {
		t.Fatal("expected error for invalid poll_interval")
	}
}

func TestParseInput_InvalidLookAhead(t *testing.T) {
	jsonStr := `{"trigger_type":"EventUpcoming","options":{"source":"test.ics","look_ahead":"bad"},"config":{}}`
	_, _, _, _, _, err := parseInput(strings.NewReader(jsonStr))
	if err == nil {
		t.Fatal("expected error for invalid look_ahead")
	}
}

func TestParseInput_InvalidJSON(t *testing.T) {
	_, _, _, _, _, err := parseInput(strings.NewReader("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
