package health

import "testing"

func TestParseRawNormalizesKnownDateTypos(t *testing.T) {
	data := ParseRaw("2029.1.28\n早上5:57 动态5.3 血压124 62 89\n\n20264.14\n早上 血压120 65 88")
	if len(data.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(data.Records))
	}
	if data.Records[0].Date != "2026-01-28" {
		t.Fatalf("unexpected first date: %s", data.Records[0].Date)
	}
	if data.Records[1].Date != "2026-04-14" {
		t.Fatalf("unexpected second date: %s", data.Records[1].Date)
	}
	if len(data.Issues) != 2 {
		t.Fatalf("expected 2 date issues, got %d", len(data.Issues))
	}
}

func TestParseRawReadsGlucoseAndPressure(t *testing.T) {
	data := ParseRaw("2026.2.1\n早上6:41 动态4.8 107 59 81\n晚上21:12 动态9.5")
	if len(data.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(data.Records))
	}
	morning := data.Records[0]
	if morning.DynamicGlucose == nil || *morning.DynamicGlucose != 4.8 {
		t.Fatalf("dynamic glucose was not parsed: %+v", morning.DynamicGlucose)
	}
	if morning.Systolic == nil || *morning.Systolic != 107 {
		t.Fatalf("loose blood pressure was not parsed: %+v", morning.Systolic)
	}
	if morning.Segment != "早上" {
		t.Fatalf("unexpected segment: %s", morning.Segment)
	}
}
