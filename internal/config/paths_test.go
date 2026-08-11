package config

import "testing"

func TestPathsFirstLocal(t *testing.T) {
	tests := []struct {
		name string
		in   Paths
		want string
	}{
		{"no mappings", Paths{}, ""},
		{
			"single mapping",
			Paths{Mappings: []Mapping{{Remote: "/downloads", Local: "/data"}}},
			"/data",
		},
		{
			"first non-empty local wins",
			Paths{Mappings: []Mapping{
				{Remote: "/downloads", Local: ""},
				{Remote: "/media", Local: "/data2"},
				{Remote: "/other", Local: "/data3"},
			}},
			"/data2",
		},
		{
			"all locals empty",
			Paths{Mappings: []Mapping{{Remote: "/downloads"}, {Remote: "/media"}}},
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.FirstLocal(); got != tt.want {
				t.Errorf("FirstLocal() = %q, want %q", got, tt.want)
			}
		})
	}
}
