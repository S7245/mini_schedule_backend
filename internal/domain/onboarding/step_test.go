package onboarding

import "testing"

func TestAllSteps_Length(t *testing.T) {
	if got := len(AllSteps()); got != 8 {
		t.Fatalf("AllSteps want 8, got %d", got)
	}
}

func TestIsValidStepKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"brand_profile", true},
		{"location", true},
		{"staff", true},
		{"course_category", true},
		{"course_template", true},
		{"entitlement_template", true},
		{"class_session", true},
		{"mini_program_qrcode", true},
		{"", false},
		{"foo", false},
		{"BRAND_PROFILE", false},
	}
	for _, c := range cases {
		if got := IsValidStepKey(c.key); got != c.want {
			t.Errorf("IsValidStepKey(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestIsSkippable(t *testing.T) {
	if IsSkippable(StepBrandProfile) {
		t.Errorf("brand_profile must not be skippable")
	}
	if IsSkippable(StepLocation) {
		t.Errorf("location must not be skippable")
	}
	for _, k := range []StepKey{StepStaff, StepCourseCategory, StepCourseTemplate, StepEntitlementTemplate, StepClassSession, StepMiniProgramQRCode} {
		if !IsSkippable(k) {
			t.Errorf("%s must be skippable", k)
		}
	}
}
