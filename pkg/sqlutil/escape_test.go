package sqlutil

import (
	"testing"

	"mysql-monitor/internal/model"
)

func TestEscapeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"user", "`user`"},
		{"order`name", "`order``name`"},
	}
	for _, tt := range tests {
		if got := EscapeIdentifier(tt.input); got != tt.want {
			t.Errorf("EscapeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEscapeStringLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"it's ok", `'it\'s ok'`},
		{"line\nbreak", `'line\nbreak'`},
	}
	for _, tt := range tests {
		if got := EscapeStringLiteral(tt.input); got != tt.want {
			t.Errorf("EscapeStringLiteral(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatSQLValue(t *testing.T) {
	if got := FormatSQLValue(nil); got != "NULL" {
		t.Errorf("FormatSQLValue(nil) = %q, want NULL", got)
	}
	if got := FormatSQLValue(123); got != "123" {
		t.Errorf("FormatSQLValue(123) = %q, want 123", got)
	}
	if got := FormatSQLValue(true); got != "1" {
		t.Errorf("FormatSQLValue(true) = %q, want 1", got)
	}
	if got := FormatSQLValue("abc"); got != "'abc'" {
		t.Errorf("FormatSQLValue(\"abc\") = %q, want 'abc'", got)
	}
}

func TestFormatColumnSpec(t *testing.T) {
	defVal := "active"
	col := &model.ColumnSchema{
		Name:          "status",
		ColumnType:    "varchar(32)",
		IsNullable:    "NO",
		ColumnDefault: &defVal,
		Comment:       "状态",
	}
	got := FormatColumnSpec(col)
	want := "`status` varchar(32) NOT NULL DEFAULT 'active' COMMENT '状态'"
	if got != want {
		t.Errorf("FormatColumnSpec() = %q, want %q", got, want)
	}
}
