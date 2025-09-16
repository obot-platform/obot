package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDedupeRevisions(t *testing.T) {
	for _, tt := range []struct {
		name      string
		revisions []string
		expected  []string
	}{
		{name: "no duplicates", revisions: []string{"a", "b", "c"}, expected: []string{"a", "b", "c"}},
		{name: "one duplicate out of order", revisions: []string{"a", "b", "c", "a"}, expected: []string{"b", "c", "a"}},
		{name: "multiple consecutive duplicates", revisions: []string{"a", "a", "a", "b", "b", "b"}, expected: []string{"a", "b"}},
		{name: "multiple duplicates out of order", revisions: []string{"a", "d", "a", "c", "b", "c", "b"}, expected: []string{"d", "a", "c", "b"}},
		{name: "multiple duplicates out of order with consecutives", revisions: []string{"a", "d", "a", "c", "b", "c", "b", "b"}, expected: []string{"d", "a", "c", "b"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, dedupeRevisions(tt.revisions))
		})
	}
}

func TestSetLatestConfigRevision(t *testing.T) {
	type expected struct {
		changed   bool
		revisions []string
	}
	for _, tt := range []struct {
		name     string
		thread   *Thread
		revision string
		expected expected
	}{
		{
			name:     "empty revision history",
			thread:   &Thread{},
			revision: "a",
			expected: expected{changed: true, revisions: []string{"a"}},
		},
		{
			name: "template thread with new revision",
			thread: &Thread{
				Spec:   ThreadSpec{Template: true},
				Status: ThreadStatus{ConfigRevisions: []string{"c", "b"}},
			},
			revision: "a",
			expected: expected{changed: true, revisions: []string{"c", "b", "a"}},
		},
		{
			name: "template thread with old latest revision",
			thread: &Thread{
				Spec:   ThreadSpec{Template: true},
				Status: ThreadStatus{ConfigRevisions: []string{"c", "b", "a"}},
			},
			revision: "a",
			expected: expected{changed: false, revisions: []string{"c", "b", "a"}},
		},
		{
			name: "template thread with duplicate revisions",
			thread: &Thread{
				Spec:   ThreadSpec{Template: true},
				Status: ThreadStatus{ConfigRevisions: []string{"b", "c", "b", "c", "a", "a", "b"}},
			},
			revision: "a",
			expected: expected{changed: true, revisions: []string{"c", "b", "a"}},
		},
		{
			name: "thread with no revisions",
			thread: &Thread{
				Spec: ThreadSpec{Project: true},
			},
			revision: "a",
			expected: expected{changed: true, revisions: []string{"a"}},
		},
		{
			name: "thread with old latest revision",
			thread: &Thread{
				Spec:   ThreadSpec{Project: true},
				Status: ThreadStatus{ConfigRevisions: []string{"b"}},
			},
			revision: "a",
			expected: expected{changed: true, revisions: []string{"a"}},
		},
		{
			name: "thread with multiple revisions keeps old latest revision",
			thread: &Thread{
				Spec:   ThreadSpec{Project: true},
				Status: ThreadStatus{ConfigRevisions: []string{"b", "c", "a"}},
			},
			revision: "a",
			expected: expected{changed: true, revisions: []string{"a"}},
		},
		{
			name: "thread with multiple revisions replaces old latest revision",
			thread: &Thread{
				Spec:   ThreadSpec{Project: true},
				Status: ThreadStatus{ConfigRevisions: []string{"b", "c", "a"}},
			},
			revision: "d",
			expected: expected{changed: true, revisions: []string{"d"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changed := tt.thread.SetLatestConfigRevision(tt.revision)
			assert.Equal(t, tt.expected.changed, changed)
			assert.Equal(t, tt.expected.revisions, tt.thread.Status.ConfigRevisions)
		})
	}
}
