package main

import "testing"

func TestCommandLooksLikeService(t *testing.T) {
	tests := []struct {
		name    string
		cmdline string
		service string
		want    bool
	}{
		{
			name:    "direct crawler bot",
			cmdline: "/Users/test/go-crawler/crawler bot",
			service: "bot",
			want:    true,
		},
		{
			name:    "caffeinate wraps crawler worker",
			cmdline: "caffeinate -i /Users/test/go-crawler/crawler worker",
			service: "worker",
			want:    true,
		},
		{
			name:    "legacy go-crawler binary",
			cmdline: "/tmp/go-build1234/b001/exe/go-crawler bot",
			service: "bot",
			want:    true,
		},
		{
			name:    "service arg mismatch",
			cmdline: "/Users/test/go-crawler/crawler worker",
			service: "bot",
			want:    false,
		},
		{
			name:    "contains service word without crawler binary",
			cmdline: "/usr/bin/python script.py bot",
			service: "bot",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commandLooksLikeService(tt.cmdline, tt.service)
			if got != tt.want {
				t.Fatalf("commandLooksLikeService(%q, %q) = %v, want %v", tt.cmdline, tt.service, got, tt.want)
			}
		})
	}
}
