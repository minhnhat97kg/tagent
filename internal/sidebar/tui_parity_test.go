package sidebar

import "testing"

func TestAvatarDefaultOff(t *testing.T) {
	c := avatarConfigFromOptions(nil)
	if c.enabled() || c.llmEnabled() {
		t.Fatal("default avatar may not call models")
	}
	c.Character = "off"
	c.LLM = "some-model"
	if c.llmEnabled() {
		t.Fatal("off avatar may not summarize")
	}
}
