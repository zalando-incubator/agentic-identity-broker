package evidence

import (
	"testing"
)

func TestForTest_RecordsOutcomeWhenTheTestEnds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EvidenceDirEnv, dir)
	t.Run("passing", func(t *testing.T) { ForTest(t, "configuration.precedence") })
	t.Run("skipped", func(t *testing.T) {
		ForTest(t, "deployment.chart")
		t.Skip("tool not installed")
	})
	records, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, record := range records {
		got[record.Example] = record.State
		if record.Location == "" || record.Spec == "" {
			t.Errorf("incomplete record: %+v", record)
		}
	}
	if got["configuration.precedence"] != StatePassed || got["deployment.chart"] != StateSkipped {
		t.Errorf("got %v", got)
	}
}

func TestForTest_DoesNothingWithoutEvidenceDirectory(t *testing.T) {
	t.Setenv(EvidenceDirEnv, "")
	ForTest(t, "configuration.precedence")
}

func TestWrite_SkippedRecordDoesNotReplaceAnOutcome(t *testing.T) {
	dir := t.TempDir()
	record := Record{Example: "a.b.c", Spec: "scenario", Location: "x_test.go:1", State: StatePassed}
	if err := Write(dir, record); err != nil {
		t.Fatal(err)
	}
	record.State = StateSkipped
	if err := Write(dir, record); err != nil {
		t.Fatal(err)
	}
	records, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].State != StatePassed {
		t.Fatalf("a later filtered-out run must not hide the outcome: %+v", records)
	}
	record.State = StateFailed
	if err := Write(dir, record); err != nil {
		t.Fatal(err)
	}
	if records, _ = ReadDir(dir); records[0].State != StateFailed {
		t.Fatalf("a later outcome must replace an earlier one: %+v", records)
	}
}
