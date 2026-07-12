# Calendar Plugin - Manual Test Cases

Minimal set of manual tests to cover the plugin surface.

## Setup

```sh
task build:calendar
mkdir -p ~/.config/recur/plugins/calendar
cp bin/plugins/calendar/* ~/.config/recur/plugins/calendar/
```

Create a test ICS file with events at known times. Replace `DTSTART`/`DTEND` values with times relative to now before each test run.

```sh
upcoming_start=$(date -d "+5 minutes" +%Y%m%dT%H%M00)
upcoming_end=$(date -d "+10 minutes" +%Y%m%dT%H%M00)
started_start=$(date -d "-1 minutes" +%Y%m%dT%H%M00)
started_end=$(date -d "+5 minutes" +%Y%m%dT%H%M00)
past_start=$(date -d "-10 minutes" +%Y%m%dT%H%M00)
past_end=$(date -d "-1 minutes" +%Y%m%dT%H%M00)
filtered_start=$(date -d "+10 minutes" +%Y%m%dT%H%M00)
filtered_end=$(date -d "+20 minutes" +%Y%m%dT%H%M00)

dtstamp=$(date -u +%Y%m%dT%H%M%SZ)

printf "
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//recur test//EN

BEGIN:VEVENT
UID:upcoming-1@test
DTSTAMP:%s
SUMMARY:Team Standup
DTSTART;TZID=America/Los_Angeles:%s
DTEND;TZID=America/Los_Angeles:%s
LOCATION:Office Room A
DESCRIPTION:Daily standup meeting
CATEGORIES:meetings,daily
END:VEVENT

BEGIN:VEVENT
UID:upcoming-2@test
DTSTAMP:%s
SUMMARY:Lunch Break
DTSTART;TZID=America/Los_Angeles:%s
DTEND;TZID=America/Los_Angeles:%s
LOCATION:Cafeteria
DESCRIPTION:Optional lunch with team
CATEGORIES:personal
END:VEVENT

BEGIN:VEVENT
UID:upcoming-3@test
DTSTAMP:%s
SUMMARY:Code Review
DTSTART;TZID=America/Los_Angeles:%s
DTEND;TZID=America/Los_Angeles:%s
DESCRIPTION:Review sprint PRs
CATEGORIES:meetings,review
END:VEVENT

BEGIN:VEVENT
UID:upcoming-4@test
DTSTAMP:%s
SUMMARY:Announcement
DTSTART;TZID=America/Los_Angeles:%s
DTEND;TZID=America/Los_Angeles:%s
DESCRIPTION: [optional] new program signup
CATEGORIES:meetings
END:VEVENT

END:VCALENDAR" "$dtstamp" "$upcoming_start" "$upcoming_end" "$dtstamp" "$started_start" "$started_end" "$dtstamp" "$past_start" "$past_end" "$dtstamp" "$filtered_start" "$filtered_end" > test.ics
```

Adjust `DTSTART`/`DTEND` so that:
- **upcoming-1** starts within the next 10 minutes (for EventUpcoming tests)
- **upcoming-2** started within the last 2 minutes (for EventStarted tests)
- **upcoming-3** ended within the last 2 minutes (for EventEnded tests)

## Test 1: EventUpcoming with local file + look_ahead + poll_interval

Covers: EventUpcoming trigger, local file source, custom look_ahead, custom poll_interval, context variables (EventTitle, EventStart, StartsIn, EventLocation).

```yaml
# ~/test-cal/recur.yaml
UpcomingMeeting:
  on:
    - type: EventUpcoming
      options:
        source: "/tmp/test-calendar.ics"
        look_ahead: "10m"
        poll_interval: "5s"
      do:
        - shell: "echo 'UPCOMING: {{.EventTitle}} starts={{.EventStart}} in={{.StartsIn}} loc={{.EventLocation}}'"
```

```sh
# Set upcoming-1 DTSTART to ~5 minutes from now before running
cd ~/test-cal && recur start --foreground &
recur register
# expect UPCOMING with title=Team Standup, StartsIn populated, Location=Office Room A
# wait past one poll_interval -- same event should NOT fire again (dedup)
```

## Test 2: EventStarted + EventEnded

Covers: EventStarted trigger, EventEnded trigger, context variables (EventUID, EventEnd, EventDescription, EventCategories).

```yaml
# ~/test-cal2/recur.yaml
LifecycleEvents:
  on:
    - type: EventStarted
      options:
        source: "/tmp/test-calendar.ics"
        poll_interval: "5s"
      do:
        - shell: "echo 'STARTED: uid={{.EventUID}} title={{.EventTitle}} desc={{.EventDescription}} cats={{.EventCategories}}'"
    - type: EventEnded
      options:
        source: "/tmp/test-calendar.ics"
        poll_interval: "5s"
      do:
        - shell: "echo 'ENDED: uid={{.EventUID}} title={{.EventTitle}} end={{.EventEnd}}'"
```

```sh
# Set upcoming-2 so it just started; upcoming-3 so it just ended
cd ~/test-cal2 && recur start --foreground &
recur register
# expect STARTED for upcoming-2 with desc=Optional lunch with team, cats=personal
# expect ENDED for upcoming-3 with end time populated
```

## Test 3: Include filters (filter_title + filter_location)

Covers: filter_title substring match, filter_location substring match, AND logic between filters, case-insensitive matching.

```yaml
# ~/test-cal3/recur.yaml
FilteredUpcoming:
  on:
    - type: EventUpcoming
      options:
        source: "/tmp/test-calendar.ics"
        look_ahead: "24h"
        poll_interval: "5s"
        filter_title: "standup"
        filter_location: "office"
      do:
        - shell: "echo 'FILTERED: {{.EventTitle}} at {{.EventLocation}}'"
```

```sh
cd ~/test-cal3 && recur start --foreground &
recur register
# expect FILTERED for "Team Standup" (title contains "standup", location contains "office")
# expect NO event for "Lunch Break" (location=Cafeteria, does not match "office")
# expect NO event for "Code Review" (no location, fails filter_location)
```

## Test 4: Exclude filters + filter_category

Covers: exclude_title, exclude_description, filter_category, exclude checked before include.

```yaml
# ~/test-cal4/recur.yaml
ExcludeAndCategory:
  on:
    - type: EventUpcoming
      options:
        source: "/tmp/test-calendar.ics"
        look_ahead: "24h"
        poll_interval: "5s"
        filter_category: "meetings"
        exclude_description: "optional"
      do:
        - shell: "echo 'PASS: {{.EventTitle}} cats={{.EventCategories}}'"
```

```sh
cd ~/test-cal4 && recur start --foreground &
recur register
# expect PASS for "Team Standup" (category=meetings, description does not contain "optional")
# expect PASS for "Code Review" (category=meetings, description does not contain "optional")
# expect NO event for "Lunch Break" (category=personal, not meetings)
# Also: Lunch Break has "Optional" in description -- excluded even if category matched
```

## Test 5: URL source + deduplication across restarts

Covers: HTTP/HTTPS source, deduplication (same event not fired twice within 2 * poll_interval), state persistence across daemon restart.

```yaml
# ~/test-cal5/recur.yaml
RemoteCal:
  on:
    - type: EventStarted
      options:
        source: "http://localhost:8080/test-calendar.ics"
        poll_interval: "5s"
      do:
        - shell: "echo 'REMOTE-STARTED: {{.EventTitle}}' >> /tmp/cal-test.log"
```

```sh
# Serve the ICS file over HTTP
python3 -m http.server 8080 --directory /tmp &

rm -f /tmp/cal-test.log
cd ~/test-cal5 && recur start --foreground &
recur register
sleep 10
# expect exactly one REMOTE-STARTED line in /tmp/cal-test.log (dedup prevents repeat)

# Stop and restart daemon
recur stop
recur start --foreground &
sleep 10
# expect still only one line (state persisted, event not re-fired)
cat /tmp/cal-test.log | wc -l  # should be 1
```

## What to verify

- [ ] All three trigger types (EventUpcoming, EventStarted, EventEnded) fire at the right time
- [ ] Local file and URL sources both work
- [ ] look_ahead window controls when EventUpcoming fires
- [ ] poll_interval controls check frequency
- [ ] Include filters (filter_title, filter_location, filter_description, filter_category) use AND logic
- [ ] Exclude filters (exclude_title, exclude_location, exclude_description) reject before include filters run
- [ ] Filters are case-insensitive
- [ ] Context variables (EventUID, EventTitle, EventStart, EventEnd, EventDescription, EventLocation, EventCategories, StartsIn) are populated
- [ ] StartsIn is only present for EventUpcoming
- [ ] Events are deduplicated by UID + start time within 2 * poll_interval
- [ ] State persists across daemon restarts
- [ ] Plugin shows up in `recur list plugins`
- [ ] `recur inspect plugin calendar` shows all 3 triggers
