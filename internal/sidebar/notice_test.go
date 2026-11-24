package sidebar

import (
	"testing"
	"time"
)

func TestNoticeExpires(t *testing.T) {
	m := Model{taskNotice: "! boom"}
	m = m.expireNotice()
	if m.taskNotice == "" {
		t.Fatal("a new notice must show")
	}
	m.noticeAt = time.Now().Add(-m.noticeFor - time.Second)
	if m = m.expireNotice(); m.taskNotice != "" {
		t.Fatal("notice outlived its time")
	}
}
