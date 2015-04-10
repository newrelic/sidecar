package output

import (
	"strconv"
	"time"
)

func TimeAgo(when time.Time, ref time.Time) string {
	diff := ref.Round(time.Second).Sub(when.Round(time.Second))

	switch {
	case when.IsZero():
		return "never"
	case diff > time.Hour*24*7:
		result := diff.Hours() / 24 / 7
		return strconv.FormatFloat(result, 'f', 1, 64) + " weeks ago"
	case diff > time.Hour*24:
		result := diff.Hours() / 24
		return strconv.FormatFloat(result, 'f', 1, 64) + " days ago"
	case diff > time.Hour:
		result := diff.Hours()
		return strconv.FormatFloat(result, 'f', 1, 64) + " hours ago"
	case diff > time.Minute:
		result := diff.Minutes()
		return strconv.FormatFloat(result, 'f', 1, 64) + " mins ago"
	case diff > time.Second:
		return strconv.FormatFloat(diff.Seconds(), 'f', 1, 64) + " secs ago"
	default:
		return "1.0 sec ago"
	}
}
