package service

import (
	"errors"
	"testing"

	"groundclearance/internal/constants"
	"groundclearance/internal/model"
	"groundclearance/internal/util"
)

func TestClearanceTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{constants.ClearancePending, constants.ClearanceCleared, true},
		{constants.ClearancePending, constants.ClearanceRestricted, true},
		{constants.ClearancePending, constants.ClearanceRevoked, true},
		{constants.ClearanceCleared, constants.ClearanceRevoked, true},
		{constants.ClearanceRestricted, constants.ClearanceRevoked, true},
		{constants.ClearanceRevoked, constants.ClearanceCleared, false},
		{constants.ClearanceCleared, constants.ClearanceRestricted, false},
	}
	for _, item := range cases {
		if got := allowedClearanceTransition(item.from, item.to); got != item.want {
			t.Fatalf("transition %s -> %s: got %v want %v", item.from, item.to, got, item.want)
		}
	}
}

func TestTurnaroundTransitionRedLines(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{constants.TurnaroundOpen, constants.TurnaroundChecking, true},
		{constants.TurnaroundDecisioned, constants.TurnaroundCompleted, true},
		{constants.TurnaroundOpen, constants.TurnaroundDecisioned, false},
		{constants.TurnaroundChecking, constants.TurnaroundCompleted, false},
		{constants.TurnaroundCompleted, constants.TurnaroundOpen, false},
	}
	for _, item := range cases {
		if got := allowedTurnaroundTransition(item.from, item.to); got != item.want {
			t.Fatalf("turnaround transition %s -> %s: got %v want %v", item.from, item.to, got, item.want)
		}
	}
}

func TestGroundUnitTransitionRedLines(t *testing.T) {
	if allowedUnitTransition(constants.UnitRetired, constants.UnitAvailable) {
		t.Fatal("retired equipment must be terminal")
	}
	if allowedUnitTransition(constants.UnitAvailable, constants.UnitAvailable) {
		t.Fatal("same-state equipment transition must be rejected")
	}
	if !allowedUnitTransition(constants.UnitBlocked, constants.UnitAvailable) {
		t.Fatal("authorized recovery from blocked state must remain possible")
	}
}

func TestNormalizeEvidence(t *testing.T) {
	items, err := normalizeEvidence([]string{" inspection-1.jpg ", "inspection-1.jpg", "meter-2.json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0] != "inspection-1.jpg" {
		t.Fatalf("unexpected normalized evidence: %#v", items)
	}
	if _, err := normalizeEvidence([]string{"   "}); err == nil {
		t.Fatal("blank evidence must be rejected")
	} else {
		var appErr *util.AppError
		if !errors.As(err, &appErr) || appErr.Code != constants.CodeValidationFailed {
			t.Fatalf("unexpected blank evidence error: %v", err)
		}
	}
}

func TestSharedEnums(t *testing.T) {
	if !constants.IsValidUnitState(constants.UnitInspection) || constants.IsValidUnitState("broken") {
		t.Fatal("unit state validation mismatch")
	}
	if !constants.IsValidRiskLevel(constants.RiskCritical) || constants.IsValidRiskLevel("urgent") {
		t.Fatal("risk validation mismatch")
	}
}

func TestReconsiderationBlockers(t *testing.T) {
	passedChecks := []model.SafetyCheck{{CheckCode: "TUG-BRAKE", Result: constants.CheckPassed}}
	availableUnits := []model.GroundUnit{{UnitCode: "TUG-017", State: constants.UnitAvailable}}

	if blockers := reconsiderationBlockers(passedChecks, availableUnits); len(blockers) != 0 {
		t.Fatalf("restored equipment and finished checks must be eligible, got blockers: %v", blockers)
	}
	if blockers := reconsiderationBlockers(nil, availableUnits); len(blockers) == 0 {
		t.Fatal("a turnaround without any checks must not be reconsiderable")
	}

	notReady := reconsiderationBlockers(
		[]model.SafetyCheck{
			{CheckCode: "TUG-BRAKE", Result: constants.CheckPending},
			{CheckCode: "GPU-INSULATION", Result: constants.CheckFailed},
		},
		[]model.GroundUnit{
			{UnitCode: "GPU-204", State: constants.UnitBlocked},
			{UnitCode: "TUG-017", State: constants.UnitAvailable},
		},
	)
	if len(notReady) != 3 {
		t.Fatalf("expected one pending, one failed and one blocked unit blocker, got: %v", notReady)
	}

	stillInspection := reconsiderationBlockers(passedChecks, []model.GroundUnit{{UnitCode: "GPU-204", State: constants.UnitInspection}})
	if len(stillInspection) != 1 {
		t.Fatalf("equipment still in inspection must block reconsideration: %v", stillInspection)
	}
}
