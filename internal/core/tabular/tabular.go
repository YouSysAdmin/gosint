// Gosint, Copyright (c) 2026 YouSysAdmin

// Package tabular reads a table out of a CSV file or the first sheet of
// an XLSX workbook: a header row of column names and the rows under
// it, every cell as text. The XLSX reader is the stdlib over the zip,
// enough for shared and inline strings, numbers and booleans. A date
// cell comes out as the serial number Excel stores.
package tabular

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

// MaxRows bounds what one file may hold.
const MaxRows = 20000

// ErrTooManyRows is a file over MaxRows.
var ErrTooManyRows = fmt.Errorf("the table has more than %d rows", MaxRows)

// MaxColumns bounds the header. Every row is padded to the header, so
// without it a wide header line over short rows asks for gigabytes.
const MaxColumns = 256

// MaxCells bounds rows times columns, what the table holds in memory.
const MaxCells = 2_000_000

// ErrTooWide is a header over MaxColumns.
var ErrTooWide = fmt.Errorf("the table has more than %d columns", MaxColumns)

// ErrTooLarge is a table over MaxCells.
var ErrTooLarge = fmt.Errorf("the table has more than %d cells", MaxCells)

// setHeader takes rec as the header, refusing one over MaxColumns.
func (t *Table) setHeader(rec []string) error {
	if len(rec) > MaxColumns {
		return ErrTooWide
	}

	t.Columns = header(rec)

	return nil
}

// addRow appends rec fitted to the header, refusing a table over
// MaxRows or MaxCells.
func (t *Table) addRow(rec []string) error {
	if len(t.Rows) >= MaxRows {
		return ErrTooManyRows
	}

	if (len(t.Rows)+1)*len(t.Columns) > MaxCells {
		return ErrTooLarge
	}

	t.Rows = append(t.Rows, fit(rec, len(t.Columns)))

	return nil
}

// Table is the header and the rows, each row as wide as the header.
type Table struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// Read parses data by the file's name: .xlsx as a workbook, anything
// else as CSV with the delimiter sniffed (comma, semicolon or tab).
func Read(name string, data []byte) (*Table, error) {
	if strings.EqualFold(path.Ext(name), ".xlsx") || bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return readXLSX(data)
	}

	return readCSV(data)
}

func readCSV(data []byte) (*Table, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sniff(data)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	var t Table
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("csv: %w", err)
		}

		if t.Columns == nil {
			if err := t.setHeader(rec); err != nil {
				return nil, err
			}

			continue
		}

		if blank(rec) {
			continue
		}

		if err := t.addRow(rec); err != nil {
			return nil, err
		}
	}

	if t.Columns == nil {
		return nil, errors.New("the file is empty")
	}

	return &t, nil
}

// sniff picks the delimiter the first line uses most among the three.
func sniff(data []byte) rune {
	line, _, _ := bytes.Cut(data, []byte("\n"))
	best, count := ',', bytes.Count(line, []byte(","))
	for _, d := range []rune{';', '\t'} {
		if n := bytes.Count(line, []byte(string(d))); n > count {
			best, count = d, n
		}
	}

	return best
}

// header is the column names trimmed, a blank one named by position
// and a repeated one numbered, so every column can be picked.
func header(rec []string) []string {
	seen := map[string]int{}
	out := make([]string, 0, len(rec))
	for i, c := range rec {
		c = strings.TrimSpace(c)
		if c == "" {
			c = fmt.Sprintf("column %d", i+1)
		}

		if n := seen[c]; n > 0 {
			c = fmt.Sprintf("%s (%d)", c, n+1)
		}

		seen[strings.TrimSpace(rec[i])]++
		out = append(out, c)
	}

	return out
}

func blank(rec []string) bool {
	for _, v := range rec {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}

	return true
}

// fit pads or cuts rec to n cells, trimmed.
func fit(rec []string, n int) []string {
	out := make([]string, n)
	for i := range out {
		if i < len(rec) {
			out[i] = strings.TrimSpace(rec[i])
		}
	}

	return out
}

// The XLSX shapes read.
type (
	sst struct {
		Items []struct {
			Text  string `xml:"t"`
			Parts []struct {
				Text string `xml:"t"`
			} `xml:"r"`
		} `xml:"si"`
	}

	sheet struct {
		Rows []struct {
			Cells []struct {
				Ref   string `xml:"r,attr"`
				Type  string `xml:"t,attr"`
				Value string `xml:"v"`
				In    struct {
					Text string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
)

func readXLSX(data []byte) (*Table, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("not an xlsx workbook")
	}

	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}

	var strs []string
	if f, ok := files["xl/sharedStrings.xml"]; ok {
		var s sst
		if err := decode(f, &s); err != nil {
			return nil, err
		}

		if len(s.Items) > maxStrings {
			return nil, fmt.Errorf("the workbook declares more than %d shared strings", maxStrings)
		}

		strs = make([]string, 0, len(s.Items))
		for _, it := range s.Items {
			var text strings.Builder
			text.WriteString(it.Text)
			for _, p := range it.Parts {
				text.WriteString(p.Text)
			}

			strs = append(strs, text.String())
		}
	}

	f, ok := files["xl/worksheets/sheet1.xml"]
	if !ok {
		return nil, errors.New("the workbook has no first sheet")
	}

	var sh sheet
	if err := decode(f, &sh); err != nil {
		return nil, err
	}

	var t Table
	for _, row := range sh.Rows {
		cells := map[int]string{}
		width := 0
		for _, c := range row.Cells {
			col := column(c.Ref)
			if col < 0 {
				continue
			}

			var v string
			switch c.Type {
			case "s":
				i, err := strconv.Atoi(c.Value)
				if err == nil && i >= 0 && i < len(strs) {
					v = strs[i]
				}
			case "inlineStr":
				v = c.In.Text
			case "b":
				switch c.Value {
				case "1":
					v = "true"
				case "0":
					v = "false"
				}
			default:
				v = c.Value
			}

			cells[col] = strings.TrimSpace(v)
			width = max(width, col+1)
		}

		rec := make([]string, width)
		for i := range rec {
			rec[i] = cells[i]
		}

		if t.Columns == nil {
			if blank(rec) {
				continue
			}

			if err := t.setHeader(rec); err != nil {
				return nil, err
			}

			continue
		}

		if blank(rec) {
			continue
		}

		if err := t.addRow(rec); err != nil {
			return nil, err
		}
	}

	if t.Columns == nil {
		return nil, errors.New("the first sheet is empty")
	}

	return &t, nil
}

// maxPart bounds one XML part of the workbook: the sheet is bounded by
// MaxRows and the shared strings by maxStrings, so a part stays small
// enough for one request.
const maxPart = 8 << 20

// maxStrings bounds the shared string table a workbook may declare.
const maxStrings = MaxRows * 256

// maxColumn is the last column Excel can address (XFD), zero based.
const maxColumn = 16384 - 1

func decode(f *zip.File, v any) error {
	r, err := f.Open()
	if err != nil {
		return err
	}

	defer r.Close() //nolint:errcheck // read-only

	if err := xml.NewDecoder(io.LimitReader(r, maxPart)).Decode(v); err != nil {
		return fmt.Errorf("xlsx: %s: %w", f.Name, err)
	}

	return nil
}

// column is the zero-based column of a cell reference such as "BC12",
// -1 when it has no letters or names a column past what Excel can
// address, so a crafted reference cannot size a row.
func column(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}

		n = n*26 + int(r-'A') + 1
		if n > maxColumn+1 {
			return -1
		}
	}

	return n - 1
}
