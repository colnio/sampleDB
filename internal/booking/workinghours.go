package booking

import (
	"fmt"
	"strings"
	"time"
)

// IsAFM reports whether the working-hours restriction applies to the named
// equipment. There is no equipment-level setting for this: any equipment whose
// name contains "AFM" (case-insensitive) counts, so "AFM-2" is covered too.
func IsAFM(equipmentName string) bool {
	return strings.Contains(strings.ToUpper(equipmentName), "AFM")
}

// WorkingWindow describes the working hours of one calendar day, taken from a
// date exception if the day has one and from the regular schedule otherwise.
type WorkingWindow struct {
	Date        string // YYYY-MM-DD
	Open        bool   // false when the day has no working hours at all
	StartMinute int    // minutes from midnight, inclusive
	EndMinute   int    // minutes from midnight, inclusive
	IsException bool
	Note        string // exception note, may be empty

	// The regular schedule, used to describe the rules to the user.
	RegularStartMinute int
	RegularEndMinute   int
	RegularWeekdays    []int32 // 0 = Sunday
}

// Contains reports whether the booking [start, end) lies entirely inside the
// window, evaluated in loc. A booking running across two days is never inside,
// as it spans the night.
func (w WorkingWindow) Contains(start, end time.Time, loc *time.Location) bool {
	if !w.Open {
		return false
	}
	start, end = start.In(loc), end.In(loc)

	y, m, d := start.Date()
	if ey, em, ed := end.Date(); ey != y || em != m || ed != d {
		return false
	}
	open := time.Date(y, m, d, 0, w.StartMinute, 0, 0, loc)
	closing := time.Date(y, m, d, 0, w.EndMinute, 0, 0, loc)
	return !start.Before(open) && !end.After(closing)
}

// Describe states the hours that apply, for the message shown to the user.
func (w WorkingWindow) Describe() string {
	if !w.IsException {
		return fmt.Sprintf("Working hours are %s, %s–%s.",
			formatWeekdays(w.RegularWeekdays), formatMinute(w.RegularStartMinute), formatMinute(w.RegularEndMinute))
	}
	note := ""
	if w.Note != "" {
		note = " (" + w.Note + ")"
	}
	if !w.Open {
		return fmt.Sprintf("There are no working hours on %s%s.", w.Date, note)
	}
	return fmt.Sprintf("Working hours on %s are %s–%s%s.", w.Date, formatMinute(w.StartMinute), formatMinute(w.EndMinute), note)
}

func formatMinute(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// formatWeekdays lists the days Monday first, as the working week is read.
func formatWeekdays(days []int32) string {
	var names []string
	for _, wd := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
		if containsWeekday(days, wd) {
			names = append(names, wd.String()[:3])
		}
	}
	return strings.Join(names, ", ")
}

func containsWeekday(days []int32, wd time.Weekday) bool {
	for _, d := range days {
		if d == int32(wd) {
			return true
		}
	}
	return false
}
