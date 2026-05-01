package haloscan

import (
	"encoding/json"
	"testing"
)

func TestNumberUnmarshal(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantValid bool
		wantValue float64
	}{
		{"int", `42`, true, 42},
		{"float", `3.14`, true, 3.14},
		{"NA string", `"NA"`, false, 0},
		{"empty string", `""`, false, 0},
		{"null", `null`, false, 0},
		{"quoted number", `"7"`, true, 7},
		{"non-numeric string", `"hello"`, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var n Number
			if err := json.Unmarshal([]byte(tc.input), &n); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if n.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", n.Valid, tc.wantValid)
			}
			if n.Value != tc.wantValue {
				t.Errorf("Value = %v, want %v", n.Value, tc.wantValue)
			}
		})
	}
}

func TestFlagUnmarshalNA(t *testing.T) {
	// The whole point: "NA" must NOT be treated as true (Python's bool("NA") trap).
	cases := []struct {
		name      string
		input     string
		wantValid bool
		wantValue bool
	}{
		{"true", `true`, true, true},
		{"false", `false`, true, false},
		{"NA string", `"NA"`, false, false},
		{"null", `null`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var f Flag
			if err := json.Unmarshal([]byte(tc.input), &f); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if f.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", f.Valid, tc.wantValid)
			}
			if f.Value != tc.wantValue {
				t.Errorf("Value = %v, want %v", f.Value, tc.wantValue)
			}
			// True() must only return true when both Valid and Value are true.
			wantTrue := tc.wantValid && tc.wantValue
			if got := f.True(); got != wantTrue {
				t.Errorf("True() = %v, want %v", got, wantTrue)
			}
		})
	}
}

func TestPositionParsesRealHaloscanRow(t *testing.T) {
	// Sample taken verbatim from data/haloscan/singular.json — the shape that
	// previously broke the Python pipeline (cpc/competition/kgr as "NA").
	raw := `{
		"position": 29,
		"traffic": 1,
		"keyword": "define workbook and worksheet",
		"allintitle": "NA",
		"result_count": 48000,
		"ads_volume": 0,
		"cpc": "NA",
		"competition": "NA",
		"si_info": true,
		"si_brand": false,
		"kvi": 22,
		"redirects_to": "NA",
		"volume": 0,
		"kgr": "NA",
		"word_count": 4,
		"url": "https://www.singular-is-future.com/post/qu-est-ce-que-vba-excel"
	}`
	var p Position
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal Position: %v", err)
	}
	if p.Position != 29 {
		t.Errorf("Position = %d, want 29", p.Position)
	}
	if p.Keyword != "define workbook and worksheet" {
		t.Errorf("Keyword = %q", p.Keyword)
	}
	if p.CPC.Valid {
		t.Errorf("CPC should be invalid (NA), got Valid=true Value=%v", p.CPC.Value)
	}
	if !p.SiInfo.True() {
		t.Error("SiInfo should be true")
	}
	if p.SiBrand.True() {
		t.Error("SiBrand should be false (Valid but Value=false)")
	}
	if p.SiBrand.Valid != true {
		t.Error("SiBrand should be Valid (was explicitly false in JSON)")
	}
}
