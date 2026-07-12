---
title: "Calendar"
weight: 4
description: "iCal/ICS calendar event triggers with filtering"
---

# Calendar Plugin

Calendar event triggers via iCal/ICS polling. Monitors a calendar source (URL or local file) and fires events when entries are upcoming, started, or ended.

## Triggers

### EventUpcoming

Fires when an event's start time is within the `look_ahead` window.

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `source` | yes | — | URL or local file path to `.ics` calendar |
| `look_ahead` | no | `15m` | How far ahead to check (Go duration) |
| `poll_interval` | no | `5m` | How often to poll the source (Go duration) |
| `filter_title` | no | — | Include only events whose title contains this substring |
| `filter_location` | no | — | Include only events whose location contains this substring |
| `filter_description` | no | — | Include only events whose description contains this substring |
| `filter_category` | no | — | Include only events with a matching category |
| `exclude_title` | no | — | Skip events whose title contains this substring |
| `exclude_location` | no | — | Skip events whose location contains this substring |
| `exclude_description` | no | — | Skip events whose description contains this substring |

### EventStarted

Fires when an event's start time has just passed (within the poll window).

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `source` | yes | — | URL or local file path to `.ics` calendar |
| `poll_interval` | no | `5m` | How often to poll the source (Go duration) |
| `filter_title` | no | — | Include only events whose title contains this substring |
| `filter_location` | no | — | Include only events whose location contains this substring |
| `filter_description` | no | — | Include only events whose description contains this substring |
| `filter_category` | no | — | Include only events with a matching category |
| `exclude_title` | no | — | Skip events whose title contains this substring |
| `exclude_location` | no | — | Skip events whose location contains this substring |
| `exclude_description` | no | — | Skip events whose description contains this substring |

### EventEnded

Fires when an event's end time has just passed (within the poll window).

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `source` | yes | — | URL or local file path to `.ics` calendar |
| `poll_interval` | no | `5m` | How often to poll the source (Go duration) |
| `filter_title` | no | — | Include only events whose title contains this substring |
| `filter_location` | no | — | Include only events whose location contains this substring |
| `filter_description` | no | — | Include only events whose description contains this substring |
| `filter_category` | no | — | Include only events with a matching category |
| `exclude_title` | no | — | Skip events whose title contains this substring |
| `exclude_location` | no | — | Skip events whose location contains this substring |
| `exclude_description` | no | — | Skip events whose description contains this substring |

## Context Variables

All three triggers provide the same context:

| Variable | Description |
|----------|-------------|
| `EventUID` | Unique identifier of the calendar event |
| `EventTitle` | Event summary/title |
| `EventStart` | Start time in RFC 3339 format |
| `EventEnd` | End time in RFC 3339 format |
| `EventDescription` | Event description (empty if not set) |
| `EventLocation` | Event location (empty if not set) |
| `EventCategories` | Comma-separated event categories (empty if not set) |
| `StartsIn` | Duration until event starts (EventUpcoming only) |

## Filtering

All three triggers support optional include and exclude filters. Filters are case-insensitive substring matches. Include filters use AND logic (all must match). Exclude filters are checked first for fast rejection.

| Option | Description |
|--------|-------------|
| `filter_title` | Include only events whose title contains this substring |
| `filter_location` | Include only events whose location contains this substring |
| `filter_description` | Include only events whose description contains this substring |
| `filter_category` | Include only events with a matching category |
| `exclude_title` | Skip events whose title contains this substring |
| `exclude_location` | Skip events whose location contains this substring |
| `exclude_description` | Skip events whose description contains this substring |

### Only fire for standup meetings

```yaml
MeetingReminder:
  on:
    - type: EventUpcoming
      options:
        source: "https://calendar.google.com/basic.ics"
        look_ahead: "10m"
        filter_title: "standup"
  do:
    - shell: "notify-send 'Standup in {{ .StartsIn }}'"
```

### Fire for all events except lunch blocks

```yaml
WorkEvents:
  on:
    - type: EventStarted
      options:
        source: "/home/user/work.ics"
        exclude_title: "lunch"
  do:
    - shell: "echo '{{ .EventTitle }} started' >> ~/work.log"
```

### Only office events, skip optional ones

```yaml
OfficeOnly:
  on:
    - type: EventUpcoming
      options:
        source: "https://example.com/team.ics"
        look_ahead: "15m"
        filter_location: "office"
        exclude_description: "optional"
  do:
    - shell: "notify-send '{{ .EventTitle }} at {{ .EventLocation }}'"
```

## Deduplication

Events are deduplicated by UID + start time. The same event will not fire twice within `2 * poll_interval`. Recurring events with different start times are treated as distinct.

> **Pick `poll_interval` close to `look_ahead`.** Dedup only suppresses repeats within `2 * poll_interval`, so an `EventUpcoming` whose start sits inside the look-ahead window across more than two polls will fire again. Example: `look_ahead: 14m`, `poll_interval: 5m` — the event is in-window for ~14 min while dedup expires after 10 min, so it fires twice. Set `poll_interval` to roughly half of `look_ahead` (or larger) to fire once per event.

## Examples

### Desktop notification before meetings

```yaml
MeetingReminder:
  on:
    - type: EventUpcoming
      options:
        source: "https://calendar.google.com/calendar/ical/user%40gmail.com/basic.ics"
        look_ahead: "10m"
        poll_interval: "2m"
  do:
    - shell: "notify-send 'Meeting Soon' '{{ .EventTitle }} starts in {{ .StartsIn }}'"
```

### Log meeting starts from a local file

```yaml
MeetingLog:
  on:
    - type: EventStarted
      options:
        source: "/home/user/calendars/work.ics"
        poll_interval: "1m"
  do:
    - shell: "echo '{{ .EventTitle }} started at {{ .EventStart }}' >> ~/meetings.log"
```

### Clean up after events end

```yaml
PostMeeting:
  on:
    - type: EventEnded
      options:
        source: "https://example.com/team.ics"
  do:
    - shell: "cleanup-recordings.sh '{{ .EventTitle }}'"
```

### Frequent polling for time-sensitive events

```yaml
Urgent:
  on:
    - type: EventUpcoming
      options:
        source: "https://example.com/oncall.ics"
        look_ahead: "5m"
        poll_interval: "30s"
  do:
    - shell: "pager-alert.sh '{{ .EventTitle }} in {{ .StartsIn }}'"
```
