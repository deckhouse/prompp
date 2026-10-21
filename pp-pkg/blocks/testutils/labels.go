package testutils

import "github.com/prometheus/prometheus/model/labels"

// LabelsMatch reports whether ls satisfies every matcher.
// Missing labels are matched against the empty string.
func LabelsMatch(ls labels.Labels, matchers []*labels.Matcher) bool {
	for _, matcher := range matchers {
		if !matcher.Matches(ls.Get(matcher.Name)) {
			return false
		}
	}

	return true
}
