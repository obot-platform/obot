package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlicesEqualIgnoreOrder(t *testing.T) {
	tests := []struct {
		name     string
		a        []int
		b        []int
		expected bool
	}{
		{
			name:     "empty slices",
			a:        []int{},
			b:        []int{},
			expected: true,
		},
		{
			name:     "nil slices",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "nil and empty slice",
			a:        nil,
			b:        []int{},
			expected: true,
		},
		{
			name:     "identical slices same order",
			a:        []int{1, 2, 3},
			b:        []int{1, 2, 3},
			expected: true,
		},
		{
			name:     "identical slices different order",
			a:        []int{3, 1, 2},
			b:        []int{1, 2, 3},
			expected: true,
		},
		{
			name:     "different lengths",
			a:        []int{1, 2},
			b:        []int{1, 2, 3},
			expected: false,
		},
		{
			name:     "same length different elements",
			a:        []int{1, 2, 3},
			b:        []int{1, 2, 4},
			expected: false,
		},
		{
			name:     "duplicates same count",
			a:        []int{1, 1, 2, 3},
			b:        []int{3, 2, 1, 1},
			expected: true,
		},
		{
			name:     "duplicates different count",
			a:        []int{1, 1, 2, 3},
			b:        []int{1, 2, 2, 3},
			expected: false,
		},
		{
			name:     "all same elements",
			a:        []int{1, 1, 1},
			b:        []int{1, 1, 1},
			expected: true,
		},
		{
			name:     "single element equal",
			a:        []int{42},
			b:        []int{42},
			expected: true,
		},
		{
			name:     "single element not equal",
			a:        []int{42},
			b:        []int{43},
			expected: false,
		},
		{
			name:     "one slice empty other not",
			a:        []int{},
			b:        []int{1},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SlicesEqualIgnoreOrder(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSlicesEqualIgnoreOrder_Strings(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "empty string slices",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "identical string slices same order",
			a:        []string{"foo", "bar", "baz"},
			b:        []string{"foo", "bar", "baz"},
			expected: true,
		},
		{
			name:     "identical string slices different order",
			a:        []string{"baz", "foo", "bar"},
			b:        []string{"foo", "bar", "baz"},
			expected: true,
		},
		{
			name:     "different string elements",
			a:        []string{"foo", "bar"},
			b:        []string{"foo", "qux"},
			expected: false,
		},
		{
			name:     "string duplicates same count",
			a:        []string{"foo", "foo", "bar"},
			b:        []string{"bar", "foo", "foo"},
			expected: true,
		},
		{
			name:     "string duplicates different count",
			a:        []string{"foo", "foo", "bar"},
			b:        []string{"foo", "bar", "bar"},
			expected: false,
		},
		{
			name:     "empty strings included",
			a:        []string{"", "foo", ""},
			b:        []string{"foo", "", ""},
			expected: true,
		},
		{
			name:     "case sensitive comparison",
			a:        []string{"Foo", "bar"},
			b:        []string{"foo", "bar"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SlicesEqualIgnoreOrder(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSlicesEqualIgnoreOrder_ComplexTypes(t *testing.T) {
	type TestStruct struct {
		ID   int
		Name string
	}

	tests := []struct {
		name     string
		a        []TestStruct
		b        []TestStruct
		expected bool
	}{
		{
			name:     "empty struct slices",
			a:        []TestStruct{},
			b:        []TestStruct{},
			expected: true,
		},
		{
			name: "identical struct slices same order",
			a: []TestStruct{
				{ID: 1, Name: "Alice"},
				{ID: 2, Name: "Bob"},
			},
			b: []TestStruct{
				{ID: 1, Name: "Alice"},
				{ID: 2, Name: "Bob"},
			},
			expected: true,
		},
		{
			name: "identical struct slices different order",
			a: []TestStruct{
				{ID: 2, Name: "Bob"},
				{ID: 1, Name: "Alice"},
			},
			b: []TestStruct{
				{ID: 1, Name: "Alice"},
				{ID: 2, Name: "Bob"},
			},
			expected: true,
		},
		{
			name: "different struct values",
			a: []TestStruct{
				{ID: 1, Name: "Alice"},
			},
			b: []TestStruct{
				{ID: 1, Name: "Bob"},
			},
			expected: false,
		},
		{
			name: "struct duplicates same count",
			a: []TestStruct{
				{ID: 1, Name: "Alice"},
				{ID: 1, Name: "Alice"},
				{ID: 2, Name: "Bob"},
			},
			b: []TestStruct{
				{ID: 2, Name: "Bob"},
				{ID: 1, Name: "Alice"},
				{ID: 1, Name: "Alice"},
			},
			expected: true,
		},
		{
			name: "struct duplicates different count",
			a: []TestStruct{
				{ID: 1, Name: "Alice"},
				{ID: 1, Name: "Alice"},
			},
			b: []TestStruct{
				{ID: 1, Name: "Alice"},
				{ID: 2, Name: "Bob"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SlicesEqualIgnoreOrder(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}
