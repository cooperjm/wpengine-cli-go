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

func TestDoctorCountsEmpty(t *testing.T) {
	passed, warned, failed := doctorCounts(nil)
	if passed != 0 || warned != 0 || failed != 0 {
		t.Errorf("expected 0, 0, 0 for nil input, got %d, %d, %d", passed, warned, failed)
	}

	passed, warned, failed = doctorCounts([]doctorCheck{})
	if passed != 0 || warned != 0 || failed != 0 {
		t.Errorf("expected 0, 0, 0 for empty input, got %d, %d, %d", passed, warned, failed)
	}
}

func TestDoctorCountsUnknownStatus(t *testing.T) {
	checks := []doctorCheck{{Status: "unknown"}}
	passed, warned, failed := doctorCounts(checks)
	if passed != 0 || warned != 0 || failed != 0 {
		t.Errorf("expected 0, 0, 0 for unknown status, got %d, %d, %d", passed, warned, failed)
	}
}
