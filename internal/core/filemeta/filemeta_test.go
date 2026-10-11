// Gosint, Copyright (c) 2026 YouSysAdmin

package filemeta_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/filemeta"
)

// tiffWith builds a little-endian TIFF with IFD0 (make, model, the GPS
// pointer, the Exif pointer), an Exif IFD with the original date and a
// GPS IFD with a position south and west.
func tiffWith(t *testing.T) []byte {
	t.Helper()

	var b bytes.Buffer
	le := binary.LittleEndian
	w := func(v any) { _ = binary.Write(&b, le, v) }
	u32 := func(n int) uint32 { return uint32(n) } //nolint:gosec // small test layout numbers
	// Header, IFD0 at 8.
	b.WriteString("II*\x00")
	w(uint32(8))
	// IFD0: 4 entries.
	w(uint16(4))
	// IFD0 at 8 is 54 bytes, the Exif IFD at 62 is 18, the GPS IFD at 80
	// is 66, so the strings and rationals sit from 146 on.
	data := bytes.Buffer{}
	str := func(tag uint16, s string) {
		w(tag)
		w(uint16(2))
		w(u32(len(s) + 1))
		w(u32(146 + data.Len()))
		data.WriteString(s + "\x00")
	}
	str(0x010F, "Canon")
	str(0x0110, "EOS R6")
	w(uint16(0x8769))
	w(uint16(4))
	w(uint32(1))
	w(uint32(62))
	w(uint16(0x8825))
	w(uint16(4))
	w(uint32(1))
	w(uint32(80))
	w(uint32(0))
	// Exif IFD at 62: DateTimeOriginal.
	w(uint16(1))
	w(uint16(0x9003))
	w(uint16(2))
	w(uint32(20))
	w(u32(146 + data.Len()))
	data.WriteString("2026:10:08 14:05:00\x00")
	w(uint32(0))
	// GPS IFD at 80: refs and coordinates as rationals.
	w(uint16(5))
	w(uint16(0x0001))
	w(uint16(2))
	w(uint32(2))
	b.WriteString("S\x00\x00\x00")
	rat := func(tag uint16, vals ...uint32) {
		w(tag)
		w(uint16(5))
		w(u32(len(vals) / 2))
		w(u32(146 + data.Len()))
		for _, v := range vals {
			_ = binary.Write(&data, le, v)
		}
	}
	rat(0x0002, 33, 1, 51, 1, 36, 1)
	w(uint16(0x0003))
	w(uint16(2))
	w(uint32(2))
	b.WriteString("W\x00\x00\x00")
	rat(0x0004, 70, 1, 30, 1, 0, 1)
	rat(0x0006, 1200, 10)
	w(uint32(0))
	if b.Len() != 146 {
		t.Fatalf("layout drifted: %d", b.Len())
	}

	b.Write(data.Bytes())

	return b.Bytes()
}

func TestEXIFOutOfAJPEG(t *testing.T) {
	payload := append([]byte("Exif\x00\x00"), tiffWith(t)...)
	var jpg bytes.Buffer
	jpg.Write([]byte{0xFF, 0xD8, 0xFF, 0xE1})
	_ = binary.Write(&jpg, binary.BigEndian, uint16(len(payload)+2)) //nolint:gosec // a short test segment
	jpg.Write(payload)
	jpg.Write([]byte{0xFF, 0xD9})

	m := filemeta.Extract("shot.jpg", "image/jpeg", jpg.Bytes())
	if m["camera_make"] != "Canon" || m["camera_model"] != "EOS R6" || m["taken_at"] != "2026-10-08T14:05:00Z" {
		t.Errorf("camera and time: %v", m)
	}

	if m["latitude"] != -33.86 || m["longitude"] != -70.5 || m["altitude"] != 120.0 {
		t.Errorf("position: %v", m)
	}
}

func TestPDFInfo(t *testing.T) {
	pdf := []byte("%PDF-1.4\n1 0 obj << /Type /Page >> endobj\n2 0 obj << /Type /Page >> endobj\n" +
		"3 0 obj << /Title (Quarterly \\(draft\\)) /Author (Jane Roe) /Creator (Writer) /CreationDate (D:20261008140500Z) /ModDate (D:20261009) >> endobj\n")
	m := filemeta.Extract("q.pdf", "application/pdf", pdf)
	if m["title"] != "Quarterly (draft)" || m["author"] != "Jane Roe" || m["software"] != "Writer" || m["pages"] != 2 {
		t.Errorf("pdf: %v", m)
	}

	if m["created_at"] != "2026-10-08T14:05:00Z" || m["modified_at"] != "2026-10-09T00:00:00Z" {
		t.Errorf("pdf dates: %v", m)
	}
}

func TestOfficeProperties(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	core, _ := zw.Create("docProps/core.xml")
	_, _ = core.Write([]byte(`<?xml version="1.0"?><cp:coreProperties xmlns:cp="x" xmlns:dc="y" xmlns:dcterms="z"><dc:title>Plan</dc:title><dc:creator>Jane Roe</dc:creator><cp:lastModifiedBy>John Doe</cp:lastModifiedBy><dcterms:created>2026-10-01T10:00:00Z</dcterms:created></cp:coreProperties>`))
	app, _ := zw.Create("docProps/app.xml")
	_, _ = app.Write([]byte(`<?xml version="1.0"?><Properties><Application>Microsoft Office Word</Application><Company>Example Ltd</Company><Pages>3</Pages><Words>420</Words></Properties>`))
	_ = zw.Close()

	m := filemeta.Extract("plan.docx", "", buf.Bytes())
	if m["title"] != "Plan" || m["author"] != "Jane Roe" || m["last_modified_by"] != "John Doe" || m["company"] != "Example Ltd" || m["pages"] != 3 || m["words"] != 420 || m["created_at"] != "2026-10-01T10:00:00Z" {
		t.Errorf("office: %v", m)
	}
}

func TestNothingKnownIsEmpty(t *testing.T) {
	if m := filemeta.Extract("notes.txt", "text/plain", []byte("hello")); len(m) != 0 {
		t.Errorf("text: %v", m)
	}
}
