package notifications

import (
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/pocketbase/pocketbase/tools/types"
)

// FuncMap returns the helper functions exposed to templates. Three only —
// add more on operator request, not speculatively. The same map is used for
// subject and body so an operator never has to remember which helpers work
// where.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"formatTime":  formatTime,
		"statusLabel": statusLabel,
		"pluralize":   pluralize,
	}
}

// formatTime renders a timestamp in the server's local timezone. Accepts
// time.Time, PocketBase's types.DateTime, or their string form — hooks hand
// templates whatever shape the record API produced.
func formatTime(v any) string {
	t, ok := toTime(v)
	if !ok {
		return ""
	}
	return t.Local().Format("Jan 2, 2006 3:04 PM")
}

func toTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	case types.DateTime:
		return t.Time(), !t.IsZero()
	case string:
		dt, err := types.ParseDateTime(t)
		if err != nil || dt.IsZero() {
			return time.Time{}, false
		}
		return dt.Time(), true
	}
	return time.Time{}, false
}

// statusLabel turns a status enum value into human copy: "in_progress" →
// "in progress". Unknown values pass through (underscores swapped) so a
// future status still renders something readable until templates catch up.
func statusLabel(status string) string {
	return strings.ReplaceAll(status, "_", " ")
}

// pluralize appends "s" to noun when n != 1. Enough for "ticket"/"tickets",
// "visit"/"visits" — does not handle irregulars and isn't trying to.
func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
