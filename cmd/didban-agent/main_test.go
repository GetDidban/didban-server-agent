package main

import "testing"

func TestResolveSocketURL(t *testing.T) {
	tests := map[string]string{
		"https://api.getdidban.ir":      "wss://api.getdidban.ir/api/v1/live",
		"http://localhost:3333/api":     "ws://localhost:3333/api/v1/live",
		"wss://example.com/custom/path": "wss://example.com/custom/path",
	}
	for input, expected := range tests {
		actual, err := resolveSocketURL(input)
		if err != nil || actual != expected {
			t.Fatalf("resolveSocketURL(%q) = %q, %v; want %q", input, actual, err, expected)
		}
	}
}

func TestPercentage(t *testing.T) {
	actual := percentage(cpuTime{idle: 40, total: 100}, cpuTime{idle: 60, total: 200})
	if actual != 80 {
		t.Fatalf("percentage = %v; want 80", actual)
	}
}
