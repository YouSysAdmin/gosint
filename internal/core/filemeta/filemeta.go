// Gosint, Copyright (c) 2026 YouSysAdmin

// Package filemeta reads what a file says about itself: EXIF out of a
// JPEG or TIFF (camera, when and where it was taken), the Info
// dictionary of a PDF, the core and app properties of an Office
// document (docx, xlsx, pptx), and the size of a picture. Stdlib
// only, and nothing here refuses a malformed file, it just finds less.
package filemeta

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "image/gif"  // DecodeConfig registrations
	_ "image/jpeg" //
	_ "image/png"  //
)

// MaxRead bounds what Extract looks at of a file.
const MaxRead = 16 << 20

// Keys the result may hold.
const (
	Title       = "title"
	Author      = "author"
	Subject     = "subject"
	Keywords    = "keywords"
	Description = "description"
	Software    = "software"
	Company     = "company"
	LastEditor  = "last_modified_by"
	CreatedAt   = "created_at"
	ModifiedAt  = "modified_at"
	TakenAt     = "taken_at"
	CameraMake  = "camera_make"
	CameraModel = "camera_model"
	Latitude    = "latitude"
	Longitude   = "longitude"
	Altitude    = "altitude"
	Width       = "width"
	Height      = "height"
	Pages       = "pages"
	Words       = "words"
)

// Extract reads the metadata of data, named by the file's type or
// name. Empty when the file says nothing of itself.
func Extract(name, contentType string, data []byte) map[string]any {
	if len(data) > MaxRead {
		data = data[:MaxRead]
	}

	out := map[string]any{}
	lower := strings.ToLower(name)
	switch {
	case bytes.HasPrefix(data, []byte("%PDF")):
		pdf(data, out)
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		office(data, out)
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8}):
		if app1 := jpegExif(data); app1 != nil {
			exif(app1, out)
		}

		dimensions(data, out)
	case bytes.HasPrefix(data, []byte("II*\x00")) || bytes.HasPrefix(data, []byte("MM\x00*")):
		exif(data, out)
	case strings.HasPrefix(contentType, "image/") || strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".gif"):
		dimensions(data, out)
	}

	return out
}

func dimensions(data []byte, out map[string]any) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil && cfg.Width > 0 {
		out[Width], out[Height] = cfg.Width, cfg.Height
	}
}

// jpegExif finds the Exif APP1 segment's TIFF payload.
func jpegExif(data []byte) []byte {
	i := 2
	for i+4 <= len(data) {
		if data[i] != 0xFF {
			return nil
		}

		marker := data[i+1]
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			i += 2

			continue
		}

		if marker == 0xDA || marker == 0xD9 {
			return nil
		}

		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return nil
		}

		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return seg[6:]
		}

		i += 2 + size
	}

	return nil
}

// The EXIF tags read.
const (
	tagMake             = 0x010F
	tagModel            = 0x0110
	tagSoftware         = 0x0131
	tagDateTime         = 0x0132
	tagArtist           = 0x013B
	tagDescription      = 0x010E
	tagExifIFD          = 0x8769
	tagGPSIFD           = 0x8825
	tagDateTimeOriginal = 0x9003
	tagGPSLatRef        = 0x0001
	tagGPSLat           = 0x0002
	tagGPSLonRef        = 0x0003
	tagGPSLon           = 0x0004
	tagGPSAltRef        = 0x0005
	tagGPSAlt           = 0x0006
)

// tiff is one TIFF payload with its byte order.
type tiff struct {
	data []byte
	bo   binary.ByteOrder
}

// entry is one IFD entry.
type entry struct {
	tag, typ uint16
	count    uint32
	value    []byte
}

func (t tiff) u16(off int) uint16 {
	if off+2 > len(t.data) {
		return 0
	}

	return t.bo.Uint16(t.data[off:])
}

func (t tiff) u32(off int) uint32 {
	if off+4 > len(t.data) {
		return 0
	}

	return t.bo.Uint32(t.data[off:])
}

var typeSizes = map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}

// ifd reads the entries of the directory at off.
func (t tiff) ifd(off int) []entry {
	n := int(t.u16(off))
	if n > 512 {
		return nil
	}

	var out []entry
	for i := range n {
		at := off + 2 + i*12
		if at+12 > len(t.data) {
			break
		}

		e := entry{tag: t.u16(at), typ: t.u16(at + 2), count: t.u32(at + 4)}
		size := typeSizes[e.typ] * int(e.count)
		if size <= 4 {
			e.value = t.data[at+8 : at+12]
		} else {
			start := int(t.u32(at + 8))
			if start < 0 || start+size > len(t.data) || size < 0 {
				continue
			}

			e.value = t.data[start : start+size]
		}

		out = append(out, e)
	}

	return out
}

func (t tiff) text(e entry) string {
	if e.typ != 2 {
		return ""
	}

	return strings.TrimSpace(strings.TrimRight(string(e.value[:min(len(e.value), int(e.count))]), "\x00"))
}

// rationals reads unsigned rationals.
func (t tiff) rationals(e entry) []float64 {
	if e.typ != 5 && e.typ != 10 {
		return nil
	}

	var out []float64
	for i := 0; i+8 <= len(e.value) && i < int(e.count)*8; i += 8 {
		num, den := float64(t.bo.Uint32(e.value[i:])), float64(t.bo.Uint32(e.value[i+4:]))
		if den == 0 {
			return nil
		}

		out = append(out, num/den)
	}

	return out
}

// exif reads the TIFF payload: IFD0, the Exif sub IFD and the GPS IFD.
func exif(data []byte, out map[string]any) {
	if len(data) < 8 {
		return
	}

	var bo binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return
	}

	t := tiff{data: data, bo: bo}
	first := int(t.u32(4))
	var exifIFD, gpsIFD int
	for _, e := range t.ifd(first) {
		switch e.tag {
		case tagMake:
			put(out, CameraMake, t.text(e))
		case tagModel:
			put(out, CameraModel, t.text(e))
		case tagSoftware:
			put(out, Software, t.text(e))
		case tagArtist:
			put(out, Author, t.text(e))
		case tagDescription:
			put(out, Description, t.text(e))
		case tagDateTime:
			putTime(out, ModifiedAt, exifTime(t.text(e)))
		case tagExifIFD:
			exifIFD = int(t.u32From(e))
		case tagGPSIFD:
			gpsIFD = int(t.u32From(e))
		}
	}

	if exifIFD > 0 {
		for _, e := range t.ifd(exifIFD) {
			if e.tag == tagDateTimeOriginal {
				putTime(out, TakenAt, exifTime(t.text(e)))
			}
		}
	}

	if gpsIFD > 0 {
		gps(t, t.ifd(gpsIFD), out)
	}
}

func (t tiff) u32From(e entry) uint32 {
	if len(e.value) < 4 {
		return 0
	}

	return t.bo.Uint32(e.value)
}

// gps turns degrees, minutes and seconds with their hemisphere into a
// signed coordinate.
func gps(t tiff, entries []entry, out map[string]any) {
	var lat, lon []float64
	var latRef, lonRef string
	var alt float64
	altSet, below := false, false
	for _, e := range entries {
		switch e.tag {
		case tagGPSLat:
			lat = t.rationals(e)
		case tagGPSLon:
			lon = t.rationals(e)
		case tagGPSLatRef:
			latRef = t.text(e)
		case tagGPSLonRef:
			lonRef = t.text(e)
		case tagGPSAlt:
			if r := t.rationals(e); len(r) == 1 {
				alt, altSet = r[0], true
			}
		case tagGPSAltRef:
			below = len(e.value) > 0 && e.value[0] == 1
		}
	}

	if len(lat) == 3 && len(lon) == 3 {
		la := lat[0] + lat[1]/60 + lat[2]/3600
		lo := lon[0] + lon[1]/60 + lon[2]/3600
		if latRef == "S" {
			la = -la
		}

		if lonRef == "W" {
			lo = -lo
		}

		if math.Abs(la) <= 90 && math.Abs(lo) <= 180 && (la != 0 || lo != 0) {
			out[Latitude], out[Longitude] = round(la), round(lo)
		}
	}

	if altSet {
		if below {
			alt = -alt
		}

		out[Altitude] = round(alt)
	}
}

func round(v float64) float64 { return math.Round(v*1e6) / 1e6 }

// exifTime reads "2026:10:08 14:05:00".
func exifTime(s string) time.Time {
	t, err := time.Parse("2006:01:02 15:04:05", s)
	if err != nil {
		return time.Time{}
	}

	return t
}

func put(out map[string]any, key, v string) {
	if v != "" {
		out[key] = v
	}
}

func putTime(out map[string]any, key string, t time.Time) {
	if !t.IsZero() {
		out[key] = t.UTC().Format(time.RFC3339)
	}
}

// The PDF Info dictionary keys read, as the raw bytes hold them.
var (
	pdfText = regexp.MustCompile(`/(Title|Author|Subject|Keywords|Creator|Producer)\s*\((.*?[^\\])\)`)
	pdfDate = regexp.MustCompile(`/(CreationDate|ModDate)\s*\(D:(\d{4})(\d{2})?(\d{2})?(\d{2})?(\d{2})?(\d{2})?`)
	pdfPage = regexp.MustCompile(`/Type\s*/Page[^s]`)
)

func pdf(data []byte, out map[string]any) {
	keys := map[string]string{"Title": Title, "Author": Author, "Subject": Subject, "Keywords": Keywords, "Creator": Software, "Producer": "producer"}
	for _, m := range pdfText.FindAllSubmatch(data, -1) {
		v := strings.TrimSpace(strings.NewReplacer(`\(`, "(", `\)`, ")").Replace(string(m[2])))
		if strings.HasPrefix(v, "\xfe\xff") {
			v = utf16be(v[2:])
		}

		put(out, keys[string(m[1])], v)
	}

	for _, m := range pdfDate.FindAllSubmatch(data, -1) {
		parts := []int{0, 1, 1, 0, 0, 0}
		for i := range parts {
			if n, err := strconv.Atoi(string(m[i+2])); err == nil {
				parts[i] = n
			}
		}

		t := time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], 0, time.UTC)
		putTime(out, map[string]string{"CreationDate": CreatedAt, "ModDate": ModifiedAt}[string(m[1])], t)
	}

	if n := len(pdfPage.FindAllIndex(data, -1)); n > 0 {
		out[Pages] = n
	}
}

// utf16be decodes a big-endian UTF-16 string, as a PDF writes one.
func utf16be(s string) string {
	var b strings.Builder
	for i := 0; i+1 < len(s); i += 2 {
		b.WriteRune(rune(uint16(s[i])<<8 | uint16(s[i+1])))
	}

	return b.String()
}

// The Office properties read.
type (
	coreProps struct {
		Title       string `xml:"title"`
		Creator     string `xml:"creator"`
		Subject     string `xml:"subject"`
		Description string `xml:"description"`
		Keywords    string `xml:"keywords"`
		LastBy      string `xml:"lastModifiedBy"`
		Created     string `xml:"created"`
		Modified    string `xml:"modified"`
	}

	appProps struct {
		Application string `xml:"Application"`
		Company     string `xml:"Company"`
		Pages       int    `xml:"Pages"`
		Words       int    `xml:"Words"`
	}
)

func office(data []byte, out map[string]any) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return
	}

	for _, f := range z.File {
		switch f.Name {
		case "docProps/core.xml":
			var p coreProps
			if decode(f, &p) == nil {
				put(out, Title, p.Title)
				put(out, Author, p.Creator)
				put(out, Subject, p.Subject)
				put(out, Description, p.Description)
				put(out, Keywords, p.Keywords)
				put(out, LastEditor, p.LastBy)
				putTime(out, CreatedAt, isoTime(p.Created))
				putTime(out, ModifiedAt, isoTime(p.Modified))
			}
		case "docProps/app.xml":
			var p appProps
			if decode(f, &p) == nil {
				put(out, Software, p.Application)
				put(out, Company, p.Company)
				if p.Pages > 0 {
					out[Pages] = p.Pages
				}

				if p.Words > 0 {
					out[Words] = p.Words
				}
			}
		}
	}
}

func decode(f *zip.File, v any) error {
	r, err := f.Open()
	if err != nil {
		return err
	}

	defer r.Close() //nolint:errcheck // read-only

	return xml.NewDecoder(io.LimitReader(r, 1<<20)).Decode(v)
}

func isoTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}

	return time.Time{}
}
