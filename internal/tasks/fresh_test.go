package tasks

import "testing"

func TestFreshConfig(t *testing.T) {
	for _, value := range []string{"0", "1", "7", "-1", "wrong"} {
		for _, location := range []string{"role", "status"} {
			roles := "roles: {worker: {agent: pi}}\n"
			status := "{id: build, role: worker}"
			if location == "role" {
				roles = "roles: {worker: {agent: pi, fresh_after: " + value + "}}\n"
			} else {
				status = "{id: build, role: worker, fresh_after: " + value + "}"
			}
			cfg, err := ParseBoardConfig([]byte(roles + "statuses: [{id: todo}, " + status + ", {id: blocked}, {id: done}]"))
			valid := value != "-1" && value != "wrong"
			if (err == nil) != valid {
				t.Fatalf("%s %s: %v", location, value, err)
			}
			if valid && location == "status" && cfg.Statuses[1].FreshAfter == nil {
				t.Fatal("missing override")
			}
		}
	}
}
