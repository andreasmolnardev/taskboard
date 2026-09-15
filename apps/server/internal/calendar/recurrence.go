// Package calendar expands iCalendar recurrence sets within bounded windows.
package calendar

import (
	"errors"
	"fmt"
	"strings"
	"time"

	ical "github.com/emersion/go-ical"
	rrule "github.com/teambition/rrule-go"
)

var (
	ErrInvalidWindow = errors.New("calendar: invalid expansion window")
	ErrInvalidLimit  = errors.New("calendar: max occurrences must be positive")
	ErrLimitExceeded = errors.New("calendar: occurrence limit exceeded")
)

// Occurrence is one stable expansion of an iCalendar component.
// End is exclusive. AllDay reports a DTSTART with VALUE=DATE.
type Occurrence struct {
	Start  time.Time
	End    time.Time
	AllDay bool
}

// Expand returns occurrences that overlap the half-open window [windowStart,
// windowEnd). It returns ErrLimitExceeded instead of a partial result when the
// component has more than maxOccurrences matches in the window.
func Expand(component *ical.Component, windowStart, windowEnd time.Time, maxOccurrences int) ([]Occurrence, error) {
	if !windowStart.Before(windowEnd) {
		return nil, ErrInvalidWindow
	}
	if maxOccurrences <= 0 {
		return nil, ErrInvalidLimit
	}
	if component == nil {
		return nil, errors.New("calendar: component is nil")
	}

	startProp := component.Props.Get(ical.PropDateTimeStart)
	if startProp == nil {
		return nil, errors.New("calendar: component is missing DTSTART")
	}
	start, err := startProp.DateTime(time.UTC)
	if err != nil {
		return nil, fmt.Errorf("calendar: invalid DTSTART: %w", err)
	}
	allDay := startProp.ValueType() == ical.ValueDate
	duration, allDaySpan, err := eventDuration(component, start, allDay)
	if err != nil {
		return nil, err
	}

	set, err := recurrenceSet(component, start)
	if err != nil {
		return nil, err
	}

	searchStart := windowStart.Add(-duration)
	if allDaySpan > 0 {
		searchStart = windowStart.AddDate(0, 0, -allDaySpan)
	}

	occurrences := make([]Occurrence, 0, min(maxOccurrences, 16))
	for occurrenceStart := set.After(searchStart, true); !occurrenceStart.IsZero() && occurrenceStart.Before(windowEnd); occurrenceStart = set.After(occurrenceStart, false) {
		occurrenceEnd := occurrenceStart.Add(duration)
		if allDaySpan > 0 {
			occurrenceEnd = occurrenceStart.AddDate(0, 0, allDaySpan)
		}
		if !overlaps(occurrenceStart, occurrenceEnd, windowStart, windowEnd) {
			continue
		}
		if len(occurrences) == maxOccurrences {
			return nil, ErrLimitExceeded
		}
		occurrences = append(occurrences, Occurrence{Start: occurrenceStart, End: occurrenceEnd, AllDay: allDay})
	}
	return occurrences, nil
}

func recurrenceSet(component *ical.Component, start time.Time) (*rrule.Set, error) {
	set := new(rrule.Set)
	set.DTStart(start)

	rules := component.Props[ical.PropRecurrenceRule]
	if len(rules) > 1 {
		return nil, errors.New("calendar: multiple RRULE properties are not supported")
	}
	if len(rules) == 1 {
		option, err := rrule.StrToROptionInLocation(rules[0].Value, start.Location())
		if err != nil {
			return nil, fmt.Errorf("calendar: invalid RRULE: %w", err)
		}
		option.Dtstart = start
		rule, err := rrule.NewRRule(*option)
		if err != nil {
			return nil, fmt.Errorf("calendar: invalid RRULE: %w", err)
		}
		set.RRule(rule)
	} else {
		set.RDate(start)
	}

	if err := addDates(set, component.Props[ical.PropRecurrenceDates], start.Location(), false); err != nil {
		return nil, fmt.Errorf("calendar: invalid RDATE: %w", err)
	}
	if err := addDates(set, component.Props[ical.PropExceptionDates], start.Location(), true); err != nil {
		return nil, fmt.Errorf("calendar: invalid EXDATE: %w", err)
	}
	return set, nil
}

func addDates(set *rrule.Set, props []ical.Prop, loc *time.Location, exclude bool) error {
	for _, prop := range props {
		for _, value := range strings.Split(prop.Value, ",") {
			dateProp := prop
			dateProp.Value = value
			date, err := dateProp.DateTime(loc)
			if err != nil {
				return err
			}
			if exclude {
				set.ExDate(date)
			} else {
				set.RDate(date)
			}
		}
	}
	return nil
}

func eventDuration(component *ical.Component, start time.Time, allDay bool) (time.Duration, int, error) {
	endProp := component.Props.Get(ical.PropDateTimeEnd)
	durationProp := component.Props.Get(ical.PropDuration)
	if endProp != nil && durationProp != nil {
		return 0, 0, errors.New("calendar: component has both DTEND and DURATION")
	}

	if endProp != nil {
		end, err := endProp.DateTime(start.Location())
		if err != nil {
			return 0, 0, fmt.Errorf("calendar: invalid DTEND: %w", err)
		}
		if end.Before(start) {
			return 0, 0, errors.New("calendar: DTEND is before DTSTART")
		}
		if allDay {
			span := dateSpan(start, end)
			if span < 0 {
				return 0, 0, errors.New("calendar: DTEND is before DTSTART")
			}
			return end.Sub(start), span, nil
		}
		return end.Sub(start), 0, nil
	}
	if durationProp != nil {
		duration, err := durationProp.Duration()
		if err != nil {
			return 0, 0, fmt.Errorf("calendar: invalid DURATION: %w", err)
		}
		if duration < 0 {
			return 0, 0, errors.New("calendar: negative DURATION")
		}
		return duration, 0, nil
	}
	if allDay {
		return 24 * time.Hour, 1, nil
	}
	return 0, 0, nil
}

func dateSpan(start, end time.Time) int {
	span := 0
	for date := start; date.Before(end); date = date.AddDate(0, 0, 1) {
		span++
	}
	return span
}

func overlaps(start, end, windowStart, windowEnd time.Time) bool {
	if end.Equal(start) {
		return !start.Before(windowStart) && start.Before(windowEnd)
	}
	return start.Before(windowEnd) && end.After(windowStart)
}
