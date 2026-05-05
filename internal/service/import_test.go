package service

import "testing"

func testImportColumns() map[string]int {
	header := []string{
		"name",
		"gender",
		"gender_probability",
		"age",
		"age_group",
		"country_id",
		"country_name",
		"country_probability",
	}
	cols, err := ParseProfileImportHeader(header)
	if err != nil {
		panic(err)
	}
	return cols
}

func TestValidateProfileImportRow(t *testing.T) {
	cols := testImportColumns()
	row, reason, err := ValidateProfileImportRow([]string{
		"John",
		"Male",
		"0.95",
		"28",
		"adult",
		"ng",
		"Nigeria",
		"0.87",
	}, cols)
	if err != nil {
		t.Fatalf("validate good row: %v (%s)", err, reason)
	}
	if row.Name != "john" || row.Gender != "male" || row.CountryID != "NG" {
		t.Fatalf("row was not canonicalized: %+v", row)
	}
}

func TestValidateProfileImportRowRejectsBadRows(t *testing.T) {
	cols := testImportColumns()
	tests := []struct {
		name   string
		row    []string
		reason string
	}{
		{
			name:   "missing name",
			row:    []string{"", "male", "0.95", "28", "adult", "NG", "Nigeria", "0.87"},
			reason: "missing_fields",
		},
		{
			name:   "invalid gender",
			row:    []string{"john", "unknown", "0.95", "28", "adult", "NG", "Nigeria", "0.87"},
			reason: "invalid_gender",
		},
		{
			name:   "invalid age",
			row:    []string{"john", "male", "0.95", "-1", "adult", "NG", "Nigeria", "0.87"},
			reason: "invalid_age",
		},
		{
			name:   "missing country name",
			row:    []string{"john", "male", "0.95", "28", "adult", "NG", "", "0.87"},
			reason: "missing_fields",
		},
		{
			name:   "invalid encoding",
			row:    []string{string([]byte{0xff, 0xfe}), "male", "0.95", "28", "adult", "NG", "Nigeria", "0.87"},
			reason: "malformed_csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reason, err := ValidateProfileImportRow(tt.row, cols)
			if err == nil {
				t.Fatalf("expected error")
			}
			if reason != tt.reason {
				t.Fatalf("reason = %q, want %q", reason, tt.reason)
			}
		})
	}
}
