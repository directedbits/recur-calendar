// calendar is an external trigger plugin that monitors iCal/ICS calendar
// sources and fires events when calendar entries are upcoming, started, or ended.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	sdk "github.com/directedbits/recur/pkg/plugin-sdk"
)

// pluginInput is the JSON payload read from stdin.
type pluginInput struct {
	TriggerType string         `json:"trigger_type"`
	Options     map[string]any `json:"options"`
	Config      map[string]any `json:"config"`
}

// parseInput reads stdin JSON and validates trigger options, returning
// the parsed input along with the extracted source, pollInterval, and lookAhead.
func parseInput(r io.Reader) (input *pluginInput, source string, pollInterval, lookAhead time.Duration, filter EventFilter, err error) {
	input = &pluginInput{}
	if err = json.NewDecoder(r).Decode(input); err != nil {
		return nil, "", 0, 0, EventFilter{}, fmt.Errorf("reading stdin: %w", err)
	}

	// Validate trigger type
	switch input.TriggerType {
	case "EventUpcoming", "EventStarted", "EventEnded":
		// ok
	default:
		return nil, "", 0, 0, EventFilter{}, fmt.Errorf("unsupported trigger_type: %s", input.TriggerType)
	}

	// Parse common options
	source = optString(input.Options, "source", "")
	if source == "" {
		return nil, "", 0, 0, EventFilter{}, fmt.Errorf("source option is required")
	}

	pollIntervalStr := optString(input.Options, "poll_interval", "5m")
	pollInterval, err = time.ParseDuration(pollIntervalStr)
	if err != nil {
		return nil, "", 0, 0, EventFilter{}, fmt.Errorf("invalid poll_interval %q: %w", pollIntervalStr, err)
	}

	// Parse look_ahead (only used by EventUpcoming, but harmless to parse always)
	if input.TriggerType == "EventUpcoming" {
		lookAheadStr := optString(input.Options, "look_ahead", "15m")
		lookAhead, err = time.ParseDuration(lookAheadStr)
		if err != nil {
			return nil, "", 0, 0, EventFilter{}, fmt.Errorf("invalid look_ahead %q: %w", lookAheadStr, err)
		}
	}

	// Parse filter options
	filter = EventFilter{
		Title:              strings.ToLower(optString(input.Options, "filter_title", "")),
		Location:           strings.ToLower(optString(input.Options, "filter_location", "")),
		Description:        strings.ToLower(optString(input.Options, "filter_description", "")),
		Category:           strings.ToLower(optString(input.Options, "filter_category", "")),
		ExcludeTitle:       strings.ToLower(optString(input.Options, "exclude_title", "")),
		ExcludeLocation:    strings.ToLower(optString(input.Options, "exclude_location", "")),
		ExcludeDescription: strings.ToLower(optString(input.Options, "exclude_description", "")),
	}

	return input, source, pollInterval, lookAhead, filter, nil
}

func main() {
	log.SetPrefix("calendar: ")
	log.SetFlags(0)

	input, source, pollInterval, lookAhead, filter, err := parseInput(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	// Read required env vars
	socketPath := os.Getenv("RECUR_SOCKET")
	triggerID := os.Getenv("RECUR_TRIGGER_ID")
	if socketPath == "" || triggerID == "" {
		log.Fatal("RECUR_SOCKET and RECUR_TRIGGER_ID must be set")
	}

	// Start the poller
	events, stop, err := StartPoller(source, input.TriggerType, pollInterval, lookAhead, filter)
	if err != nil {
		log.Fatalf("starting poller: %v", err)
	}
	defer stop()

	log.Printf("started: %s source=%q poll_interval=%s look_ahead=%s",
		input.TriggerType, source, pollInterval, lookAhead)

	// Connect to daemon gRPC socket
	client, err := sdk.Connect(socketPath)
	if err != nil {
		log.Fatalf("connecting to daemon: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Set up signal handler for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Event loop
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				log.Print("event channel closed, exiting")
				return
			}

			ctxVars := map[string]string{
				"EventUID":         evt.UID,
				"EventTitle":       evt.Title,
				"EventStart":       evt.Start.Format(time.RFC3339),
				"EventEnd":         evt.End.Format(time.RFC3339),
				"EventDescription": evt.Description,
				"EventLocation":    evt.Location,
				"EventCategories":  evt.Categories,
			}
			if input.TriggerType == "EventUpcoming" {
				ctxVars["StartsIn"] = evt.StartsIn.Truncate(time.Second).String()
			}

			resp, err := client.Service.ReportTriggerEvent(context.Background(), &sdk.ReportTriggerEventRequest{
				TriggerId: triggerID,
				Context:   ctxVars,
			})
			if err != nil {
				log.Printf("reporting event: %v", err)
				continue
			}
			if !resp.Accepted {
				log.Printf("event rejected: %s", resp.Error)
				continue
			}

			log.Printf("event reported: %s %q", input.TriggerType, evt.Title)

		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "received %v, shutting down\n", sig)
			return
		}
	}
}

// optString extracts a string option with a default fallback.
func optString(opts map[string]any, key, fallback string) string {
	if v, ok := opts[key].(string); ok && v != "" {
		return v
	}
	return fallback
}
