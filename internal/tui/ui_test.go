package tui

import (
	"testing"
)

// The filter matches literal substrings, not fuzzy ones: bubbles' default
// fuzzy matcher let a query whose letters merely appear in order anywhere in
// a task's name plus all of its labels count as a match, which kept most of
// the list visible for names no task has.
func TestSubstringFilter(t *testing.T) {
	targets := []string{
		"build docker image env=dev region=eu",
		"deploy staging env=dev",
		"test unit",
	}

	for _, tc := range []struct {
		term string
		want []int
	}{
		{"dedde", nil},
		{"deploy", []int{1}},
		{"DEPLOY", []int{1}},
		{"env=dev", []int{0, 1}},
		{"", []int{0, 1, 2}},
	} {
		ranks := substringFilter(tc.term, targets)

		got := make([]int, 0, len(ranks))
		for _, r := range ranks {
			got = append(got, r.Index)
		}

		if len(got) != len(tc.want) {
			t.Errorf("substringFilter(%q) = %v, want %v", tc.term, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("substringFilter(%q) = %v, want %v", tc.term, got, tc.want)
				break
			}
		}
	}
}
