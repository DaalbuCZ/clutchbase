package control

import (
	"clutchbase/internal/protocol"
	"testing"
	"time"
)

func TestBuildReportValidation(t *testing.T) {
	report := protocol.Report{Driver: "managed", State: "stopped", GameBuildInstalled: "123", GameBuildLatest: "456", GameBuildCheckedAt: time.Now().UnixMilli()}
	if !validReport(report) {
		t.Fatal("valid build report rejected")
	}
	report.Driver = "simulated"
	if validReport(report) {
		t.Fatal("simulated build report accepted")
	}
	report.Driver = "managed"
	report.GameBuildLatest = "../../bad"
	if validReport(report) {
		t.Fatal("invalid build identifier accepted")
	}
	report.GameBuildLatest = "456"
	report.GameBuildCheckedAt = 0
	if validReport(report) {
		t.Fatal("untimed build report accepted")
	}
}
