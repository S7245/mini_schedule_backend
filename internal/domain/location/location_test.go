package location

import "testing"

func TestIsValidStatus(t *testing.T) {
	if !IsValidStatus("active") {
		t.Errorf("active should be valid")
	}
	if !IsValidStatus("inactive") {
		t.Errorf("inactive should be valid")
	}
	if IsValidStatus("frozen") {
		t.Errorf("frozen should not be valid")
	}
	if IsValidStatus("") {
		t.Errorf("empty should not be valid")
	}
}
