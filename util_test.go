package main

import (
	"net/url"
	"reflect"
	"testing"
)

func TestRegexVal(t *testing.T) {
	tests := []struct {
		src      string
		regex    string
		val      string
		expected string
	}{
		{
			src:      "resource-one-two",
			regex:    `resource-(.*)-(.*)`,
			val:      "$1-$2-resource",
			expected: "one-two-resource",
		},
		{
			src:      "resource-123-456",
			regex:    `resource-(\d+)-(\d+)`,
			val:      "$2-$1",
			expected: "456-123",
		},
		{
			src:      "no-match-here",
			regex:    `resource-(.*)-(.*)`,
			val:      "resource-$1-$2",
			expected: "resource-$1-$2", // unchanged since no match
		},
		{
			src:      "a-b-c-d-e",
			regex:    `a-(b)-(c)-(d)-(e)`,
			val:      "$1:$2:$3:$4",
			expected: "b:c:d:e",
		},
		{
			src:      "x-y-z",
			regex:    `(x)-(y)-(z)`,
			val:      "first=$1, second=$2, third=$3",
			expected: "first=x, second=y, third=z",
		},
	}

	for _, tt := range tests {
		got := regexVal(tt.src, tt.regex, tt.val)
		if got != tt.expected {
			t.Errorf("regexVal(%q, %q, %q) = %q; want %q", tt.src, tt.regex, tt.val, got, tt.expected)
		}
	}
}

func TestPathElements(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "labels.key(authorization.k8s.io/decision)",
			expected: []string{"labels", "authorization.k8s.io/decision"},
		},
		{
			input:    "a.b.key(c.d.e).f",
			expected: []string{"a", "b", "c.d.e", "f"},
		},
		{
			input:    "simple.path.test",
			expected: []string{"simple", "path", "test"},
		},
	}

	for _, tt := range tests {
		got := pathElements(tt.input)
		if !reflect.DeepEqual(got, tt.expected) {
			t.Errorf("pathElements(%q) = %v; want %v", tt.input, got, tt.expected)
		}
	}
}

func TestEscLogName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No slash in log name",
			input:    "simple.log",
			expected: "simple.log",
		},
		{
			name:     "Single slash in log name",
			input:    "cloudaudit.googleapis.com/activity",
			expected: url.PathEscape("cloudaudit.googleapis.com/activity"),
		},
		{
			name:     "Multiple slashes in log name",
			input:    "folder/subfolder/logname",
			expected: url.PathEscape("folder/subfolder/logname"),
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Unicode characters with slash",
			input:    "folder/日志",
			expected: url.PathEscape("folder/日志"),
		},
		{
			name:     "Unicode characters without slash",
			input:    "日志",
			expected: "日志",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escLogName(tt.input)
			if result != tt.expected {
				t.Errorf("escLogName(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}
