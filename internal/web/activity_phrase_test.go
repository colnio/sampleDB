package web

import (
	"strings"
	"testing"
)

func TestActivityPhraseHTML_SampleUpdate(t *testing.T) {
	id42 := 42
	cases := []struct {
		name    string
		label   string
		id      *int
		wantIn  []string
		wantOut []string
	}{
		{
			name:    "owner change",
			label:   "changed owner to alice",
			id:      &id42,
			wantIn:  []string{"changed owner to alice", "SMP-042", `href="/samples/42"`},
			wantOut: []string{"edited sample changed"},
		},
		{
			name:    "status change",
			label:   "moved sample to in progress state",
			id:      &id42,
			wantIn:  []string{"moved sample to in progress state", "SMP-042"},
			wantOut: []string{"edited sample moved"},
		},
		{
			name:    "description edit",
			label:   "edited description",
			id:      &id42,
			wantIn:  []string{"edited description", "SMP-042"},
			wantOut: nil,
		},
		{
			name:    "empty label falls back",
			label:   "",
			id:      &id42,
			wantIn:  []string{"edited sample details", "SMP-042"},
			wantOut: nil,
		},
		{
			name:    "no id renders plain text",
			label:   "edited description",
			id:      nil,
			wantIn:  []string{"edited description"},
			wantOut: []string{`href=`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(activityPhraseHTML("updated", "sample", tc.label, tc.id))
			for _, want := range tc.wantIn {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %q", want, got)
				}
			}
			for _, unwanted := range tc.wantOut {
				if strings.Contains(got, unwanted) {
					t.Errorf("unexpected %q in %q", unwanted, got)
				}
			}
		})
	}
}

func TestActivityPhraseHTML_OtherVerbs(t *testing.T) {
	id5 := 5
	cases := []struct {
		verb       string
		targetType string
		label      string
		id         *int
		wantIn     []string
	}{
		{"created", "sample", "GaAs Wafer", &id5, []string{"created sample", "GaAs Wafer", `href="/samples/5"`}},
		{"deleted", "sample", "Old Sample", &id5, []string{"deleted sample", "Old Sample"}},
		{"created", "article", "Cleanroom Safety", &id5, []string{"published wiki article", "Cleanroom Safety", `href="/wiki/view/Cleanroom%20Safety"`}},
		{"updated", "article", "Cleanroom Safety", &id5, []string{"edited article", "Cleanroom Safety"}},
	}

	for _, tc := range cases {
		t.Run(tc.verb+"/"+tc.targetType, func(t *testing.T) {
			got := string(activityPhraseHTML(tc.verb, tc.targetType, tc.label, tc.id))
			for _, want := range tc.wantIn {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %q", want, got)
				}
			}
		})
	}
}
