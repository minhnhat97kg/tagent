package tasks

import "encoding/json"

// UnmarshalJSON accepts the obsolete flat output map without reviving old runs.
func (t *Task) UnmarshalJSON(b []byte) error {
	type plain Task
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if value, ok := raw["outputs"]; ok {
		var nested map[string]map[string]string
		if json.Unmarshal(value, &nested) != nil {
			delete(raw, "outputs")
		}
	}
	clean, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(clean, (*plain)(t))
}
