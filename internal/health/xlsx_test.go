package health

import "testing"

func TestXLSXRoundTrip(t *testing.T) {
	dynamic := 5.8
	finger := 4.9
	systolic := 118
	diastolic := 68
	pulse := 82
	records := []Record{
		{
			Date:           "2026-04-29",
			Time:           "06:30",
			Segment:        "早上",
			DynamicGlucose: &dynamic,
			FingerGlucose:  &finger,
			Systolic:       &systolic,
			Diastolic:      &diastolic,
			Pulse:          &pulse,
			Note:           "测试",
			Source:         "manual",
		},
	}

	data, err := ExportXLSX(records)
	if err != nil {
		t.Fatalf("export xlsx: %v", err)
	}
	imported, err := ImportXLSX(data)
	if err != nil {
		t.Fatalf("import xlsx: %v", err)
	}
	if len(imported) != 1 {
		t.Fatalf("expected 1 imported record, got %d", len(imported))
	}
	got := imported[0]
	if got.Date != "2026-04-29" || got.Time != "06:30" || got.Segment != "早上" {
		t.Fatalf("unexpected imported record: %+v", got)
	}
	if got.DynamicGlucose == nil || *got.DynamicGlucose != dynamic {
		t.Fatalf("dynamic glucose mismatch: %+v", got.DynamicGlucose)
	}
	if got.Systolic == nil || *got.Systolic != systolic {
		t.Fatalf("systolic mismatch: %+v", got.Systolic)
	}
}
