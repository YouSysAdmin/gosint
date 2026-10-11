// Gosint, Copyright (c) 2026 YouSysAdmin

package tabular_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/tabular"
)

func TestCSVWithAnySeparatorAndABOM(t *testing.T) {
	for _, src := range []string{
		"\xef\xbb\xbfName,Email,Phone\nJane Roe,jane@example.com,\"+33 6 12 34 56 78\"\n\n\"Doe, John\",john@example.com\n",
		"Name;Email;Phone\nJane Roe;jane@example.com;+33 6 12 34 56 78\n\"Doe, John\";john@example.com;\n",
		"Name\tEmail\tPhone\nJane Roe\tjane@example.com\t+33 6 12 34 56 78\nDoe, John\tjohn@example.com\n",
	} {
		tb, err := tabular.Read("people.csv", []byte(src))
		if err != nil {
			t.Fatalf("%q: %v", src[:12], err)
		}

		if !slices.Equal(tb.Columns, []string{"Name", "Email", "Phone"}) || len(tb.Rows) != 2 {
			t.Fatalf("%q: columns %v, %d rows", src[:12], tb.Columns, len(tb.Rows))
		}

		if !slices.Equal(tb.Rows[1], []string{"Doe, John", "john@example.com", ""}) {
			t.Errorf("%q: row 2 = %q", src[:12], tb.Rows[1])
		}
	}
}

func TestHeaderNamesBlankAndRepeatedColumns(t *testing.T) {
	tb, err := tabular.Read("x.csv", []byte("a,,a,b\n1,2,3,4\n"))
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(tb.Columns, []string{"a", "column 2", "a (2)", "b"}) {
		t.Errorf("columns = %q", tb.Columns)
	}
}

// A workbook made by hand: shared strings, an inline string, a number
// and a boolean, with a blank row and a cell out of order.
func TestXLSXFirstSheet(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}

	add("xl/sharedStrings.xml", `<?xml version="1.0"?><sst><si><t>Domain</t></si><si><r><t>Alive</t></r><t></t></si><si><t>example.com</t></si></sst>`)
	add("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>Hits</t></is></c></row>
<row r="2"></row>
<row r="3"><c r="C3"><v>42</v></c><c r="A3" t="s"><v>2</v></c><c r="B3" t="b"><v>1</v></c></row>
</sheetData></worksheet>`)
	_ = zw.Close()

	tb, err := tabular.Read("hosts.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(tb.Columns, []string{"Domain", "Alive", "Hits"}) || len(tb.Rows) != 1 ||
		!slices.Equal(tb.Rows[0], []string{"example.com", "true", "42"}) {
		t.Errorf("table = %+v", tb)
	}
}

// A cell reference past Excel's last column is dropped instead of
// sizing the row by it.
func TestXLSXDropsACellPastTheLastColumn(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(body))
	}

	add("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet><sheetData>
<row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c><c r="ZZZZZZZ1" t="inlineStr"><is><t>far</t></is></c></row>
<row r="2"><c r="A2"><v>1</v></c><c r="XFE2"><v>2</v></c></row>
</sheetData></worksheet>`)
	_ = zw.Close()

	tb, err := tabular.Read("wide.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(tb.Columns, []string{"Name"}) || len(tb.Rows) != 1 || !slices.Equal(tb.Rows[0], []string{"1"}) {
		t.Errorf("table = %+v", tb)
	}
}

// A header wider than MaxColumns is refused before any row is padded to
// it: a wide header over short rows would otherwise ask for gigabytes.
func TestAWideTableIsRefused(t *testing.T) {
	wide := strings.Repeat(",", tabular.MaxColumns) + "\n" + strings.Repeat("x\n", 10)
	if _, err := tabular.Read("t.csv", []byte(wide)); !errors.Is(err, tabular.ErrTooWide) {
		t.Errorf("wide csv: %v", err)
	}

	cols := tabular.MaxColumns
	rows := tabular.MaxCells/cols + 2
	var b strings.Builder
	b.WriteString(strings.Repeat("h,", cols-1) + "h\n")
	for range rows {
		b.WriteString("x\n")
	}

	if _, err := tabular.Read("t.csv", []byte(b.String())); !errors.Is(err, tabular.ErrTooLarge) {
		t.Errorf("large csv: %v", err)
	}
}
