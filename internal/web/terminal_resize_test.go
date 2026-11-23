package web

import "testing"

func TestControlClientsOnly(t *testing.T) {
	for _, tt := range []struct {
		name, clients string
		ok, want      bool
	}{
		{"no clients", "", true, true},
		{"one browser", "1", true, true},
		{"multiple controls", "1\n1\n", true, true},
		{"real terminal", "0", true, false},
		{"mixed clients", "1\n0\n1", true, false},
		{"failed query", "", false, false},
		{"unknown flag", "unknown", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := controlClientsOnly(tt.clients, tt.ok); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestParseHandoffRelease(t *testing.T) {
	for _, data := range []string{`{"type":"handoff","cols":80,"rows":24}`, `{"type":"release"}`} {
		size, err := parseTerminalResize([]byte(data))
		if err != nil || (size.Action != "handoff" && size.Action != "release") {
			t.Fatalf("%+v %v", size, err)
		}
	}
}

func TestParseTerminalResize(t *testing.T) {
	for _, tt := range []struct {
		data       string
		cols, rows int
	}{
		{`{"type":"resize","cols":100,"rows":35}`, 100, 35},
		{`{"type":"resize","cols":1,"rows":1}`, 40, 10},
		{`{"type":"resize","cols":9999,"rows":9999}`, 300, 120},
	} {
		got, err := parseTerminalResize([]byte(tt.data))
		if err != nil || got.Cols != tt.cols || got.Rows != tt.rows {
			t.Fatalf("%s: %+v %v", tt.data, got, err)
		}
	}
	for _, data := range []string{``, `null`, `{`, `{"type":"input","cols":80,"rows":24}`, `{"type":"resize","cols":80}`, `{"type":"resize","cols":0,"rows":24}`, `{"type":"resize","cols":-1,"rows":24}`, `{"type":"resize","cols":80.5,"rows":24}`, `{"type":"resize","cols":"80","rows":24}`, `{"type":"resize","cols":80,"rows":24} {}`} {
		if _, err := parseTerminalResize([]byte(data)); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}
