package waitlist

import "testing"

func TestIsValidStatus(t *testing.T) {
	for _, s := range []string{"waiting", "eligible_to_promote", "promoted", "cancelled", "skipped"} {
		if !IsValidStatus(s) {
			t.Errorf("IsValidStatus(%q)=false, want true", s)
		}
	}
	if IsValidStatus("bogus") {
		t.Error("IsValidStatus(bogus)=true, want false")
	}
}
