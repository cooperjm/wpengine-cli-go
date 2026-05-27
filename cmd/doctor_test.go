package cmd

import "testing"

func TestDoctorCounts(t *testing.T) {
	checks := []doctorCheck{
		{Status: "ok"},
		{Status: "ok"},
		{Status: "warn"},
		{Status: "fail"},
	}
	passed, warned, failed := doctorCounts(checks)
	if passed != 2 {
		t.Errorf("expected 2 passed, got %d", passed)
	}
	if warned != 1 {
		t.Errorf("expected 1 warned, got %d", warned)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
}
