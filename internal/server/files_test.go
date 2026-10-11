// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	"github.com/yousysadmin/gosint/internal/domain/auth"
)

// upload sends one multipart file to a node.
func (a *api) upload(sketch, node, name, content string, out any) int {
	a.t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		a.t.Fatal(err)
	}

	_, _ = part.Write([]byte(content))
	_ = mw.Close()

	return a.do("POST", sketch+"/nodes/"+node+"/files", body.Bytes(), out, "Content-Type", mw.FormDataContentType())
}

// fetchRaw fetches a path and returns the status, body and headers.
func (a *api) fetchRaw(path string, headers ...string) (int, []byte, http.Header) {
	a.t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Cookie", auth.CookieName+"="+a.cookie)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	resp, err := a.srv.App().Test(req, testConfig)
	if err != nil {
		a.t.Fatal(err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	b, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, b, resp.Header
}

// A file attached to an entity is listed, served inline when it is an
// image, carried by the export, and its bytes go when the entity does.
func TestFilesOnAnEntity(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	var msg struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes",
		map[string]any{"type": "message", "label": "m-1", "properties": map[string]any{"content": "hello"}}, &msg)

	var up struct {
		File struct {
			ID          string `json:"id"`
			ContentType string `json:"content_type"`
			Hash        string `json:"hash"`
			URL         string `json:"url"`
			IsImage     bool   `json:"is_image"`
		} `json:"file"`
	}

	png := "\x89PNG\r\n\x1a\nnot really a picture"
	if code := alice.upload(sketch, msg.Node.ID, "shot.png", png, &up); code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}

	if up.File.ContentType != "image/png" || !up.File.IsImage || !strings.HasSuffix(up.File.URL, "/files/"+up.File.ID) {
		t.Errorf("uploaded file = %+v", up.File)
	}

	if code := alice.upload(sketch, msg.Node.ID, ".hidden.png", png, nil); code != http.StatusBadRequest {
		t.Errorf("a hidden name = %d, want 400", code)
	}

	// Served inline with the sniffed type, a range honoured.
	code, body, h := alice.fetchRaw(up.File.URL)
	if code != http.StatusOK || string(body) != png || h.Get("Content-Type") != "image/png" ||
		!strings.HasPrefix(h.Get("Content-Disposition"), "inline") {
		t.Errorf("serve = %d %q %v", code, body, h)
	}

	code, body, h = alice.fetchRaw(up.File.URL, "Range", "bytes=0-3")
	if code != http.StatusPartialContent || string(body) != png[:4] || h.Get("Content-Range") == "" {
		t.Errorf("range = %d %q %v", code, body, h)
	}

	// The last bytes by a suffix range, a header that does not parse is
	// ignored, a start past the end is refused.
	code, body, _ = alice.fetchRaw(up.File.URL, "Range", "bytes=-4")
	if code != http.StatusPartialContent || string(body) != png[len(png)-4:] {
		t.Errorf("suffix range = %d %q", code, body)
	}

	for _, bad := range []string{"bytes=abc", "bytes=0-1,4-5", "items=0-3"} {
		if code, body, _ = alice.fetchRaw(up.File.URL, "Range", bad); code != http.StatusOK || string(body) != png {
			t.Errorf("range %q = %d %q, want the whole file", bad, code, body)
		}
	}

	if code, _, _ = alice.fetchRaw(up.File.URL, "Range", "bytes=99999-"); code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("a start past the end = %d", code)
	}

	code, _, h = alice.fetchRaw(up.File.URL + "?download=1")
	if code != http.StatusOK || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") {
		t.Errorf("download = %d %v", code, h)
	}

	// A file is not drawn on the node by itself, the person picks one as
	// the picture, which is the style's image.
	var graph struct {
		Nodes []struct {
			ID    string `json:"id"`
			Image string `json:"image"`
		} `json:"nodes"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)
	if len(graph.Nodes) != 1 || graph.Nodes[0].Image != "" {
		t.Errorf("node image = %+v, want none until chosen", graph.Nodes)
	}

	// The export carries the bytes, and importing it elsewhere restores
	// the file on the right node.
	var export map[string]any
	alice.expect(http.StatusOK, "POST", sketch+"/export", map[string]any{}, &export)
	files, _ := export["files"].([]any)
	if len(files) != 1 {
		t.Fatalf("export files = %v", export["files"])
	}

	other, _ := sketchFor(t, alice)

	var result struct {
		NodesCreated int      `json:"nodes_created"`
		FilesCreated int      `json:"files_created"`
		Errors       []string `json:"errors"`
	}

	alice.expect(http.StatusOK, "POST", other+"/import",
		map[string]any{"nodes": export["nodes"], "edges": export["edges"], "files": export["files"]}, &result)
	if result.NodesCreated != 1 || result.FilesCreated != 1 || len(result.Errors) != 0 {
		t.Errorf("import = %+v", result)
	}

	// Two records share the blob. Deleting one node keeps the bytes,
	// deleting the other releases them.
	alice.expect(http.StatusOK, "POST", sketch+"/nodes/delete", map[string]any{"ids": []string{msg.Node.ID}})
	if ok, _ := rt.Blobs.Exists(t.Context(), up.File.Hash); !ok {
		t.Error("the blob went while another sketch still names it")
	}

	alice.expect(http.StatusNotFound, "GET", up.File.URL, nil)
	alice.expect(http.StatusOK, "POST", other+"/clear", nil)

	// The last record gone, the bytes are let go: kept until the
	// retention sweep, which deletes them.
	if ok, _ := rt.Blobs.Exists(t.Context(), up.File.Hash); !ok {
		t.Error("the blob went before the retention sweep")
	}

	if n, err := attachment.Sweep(t.Context(), rt, time.Now().UTC().Add(time.Hour)); n != 1 || err != nil {
		t.Errorf("the sweep deleted %d, %v, want the one blob", n, err)
	}

	if ok, _ := rt.Blobs.Exists(t.Context(), up.File.Hash); ok {
		t.Error("the blob stayed after the sweep")
	}
}

// An attached .eml opens as a message: the parsed shape in message
// order, each part on its own by the file rule, and a non-member sees
// nothing.
func TestAnEmailFileOpensAsMail(t *testing.T) {
	srv, _ := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")
	sketch, _ := sketchFor(t, alice)

	var n struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "email", "label": "jane@example.com"}, &n)

	png := "\x89PNG\r\n\x1a\nnot really a png"
	pdf := "%PDF-1.4 tiny"
	eml := strings.Join([]string{
		"Received: from relay.example (relay.example [192.0.2.2])",
		"\tby mx.example; Mon, 1 Jan 2024 10:00:00 +0000",
		"Received: from sender.example by relay.example",
		"From: Jane Roe <jane@example.com>",
		"To: bob@example.com",
		"Subject: =?UTF-8?Q?Caf=C3=A9?=",
		"Message-Id: <abc@example.com>",
		"Date: Mon, 1 Jan 2024 09:59:00 +0000",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=\"outer\"",
		"",
		"--outer",
		"Content-Type: multipart/alternative; boundary=\"inner\"",
		"",
		"--inner",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello Bob",
		"--inner",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Hello <b>Bob</b></p><img src=\"cid:logo\">",
		"--inner--",
		"--outer",
		"Content-Type: image/png",
		"Content-Disposition: inline",
		"Content-ID: <logo>",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte(png)),
		"--outer",
		"Content-Type: application/pdf",
		"Content-Disposition: attachment; filename=\"r.pdf\"",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte(pdf)),
		"--outer",
		"Content-Type: text/html; name=\"page.html\"",
		"",
		"<script>alert(1)</script>",
		"--outer--",
		"",
	}, "\r\n")

	var up struct {
		File struct {
			ID          string `json:"id"`
			ContentType string `json:"content_type"`
			URL         string `json:"url"`
		} `json:"file"`
	}

	if code := alice.upload(sketch, n.Node.ID, "note.eml", eml, &up); code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}

	if up.File.ContentType != "message/rfc822" {
		t.Errorf("content type = %q, want message/rfc822", up.File.ContentType)
	}

	var got struct {
		Mail struct {
			From      string   `json:"from"`
			FromName  string   `json:"from_name"`
			To        []string `json:"to"`
			Subject   string   `json:"subject"`
			MessageID string   `json:"message_id"`
			Date      string   `json:"date"`
			Received  []string `json:"received"`
			Headers   []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			Text        string `json:"text"`
			HTML        string `json:"html"`
			Attachments []struct {
				Index       int    `json:"index"`
				Name        string `json:"name"`
				ContentType string `json:"content_type"`
				ContentID   string `json:"content_id"`
				Size        int64  `json:"size"`
				URL         string `json:"url"`
			} `json:"attachments"`
		} `json:"mail"`
	}

	alice.expect(http.StatusOK, "GET", up.File.URL+"/mail", nil, &got)
	m := got.Mail
	if m.Subject != "Café" || m.From != "jane@example.com" || m.FromName != "Jane Roe" || m.MessageID != "abc@example.com" ||
		len(m.To) != 1 || !strings.HasPrefix(m.Date, "2024-01-01") {
		t.Errorf("envelope = %+v", m)
	}

	if len(m.Received) != 2 || !strings.HasPrefix(m.Received[0], "from relay.example") {
		t.Errorf("received = %q", m.Received)
	}

	if len(m.Headers) < 5 || m.Headers[0].Name != "Received" || m.Headers[1].Name != "Received" || m.Headers[2].Name != "From" ||
		m.Headers[4].Value != "Café" {
		t.Errorf("headers lost their order: %+v", m.Headers)
	}

	if !strings.Contains(m.Text, "Hello Bob") || !strings.Contains(m.HTML, "cid:logo") {
		t.Errorf("bodies = %q / %q", m.Text, m.HTML)
	}

	if len(m.Attachments) != 3 || m.Attachments[0].ContentID != "logo" || m.Attachments[0].ContentType != "image/png" ||
		m.Attachments[1].Name != "r.pdf" || m.Attachments[1].ContentID != "" || m.Attachments[1].Size != int64(len(pdf)) ||
		m.Attachments[2].Name != "page.html" {
		t.Errorf("attachments = %+v", m.Attachments)
	}

	// Parts come back as the message carried them, inline only by the
	// file rule: a png may render, a pdf asked for as a download
	// downloads, an html part never renders on this origin.
	code, body, hdr := alice.fetchRaw(m.Attachments[0].URL)
	if code != http.StatusOK || string(body) != png || hdr.Get("Content-Type") != "image/png" ||
		!strings.HasPrefix(hdr.Get("Content-Disposition"), "inline") || hdr.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("png part = %d %q %q %q", code, hdr.Get("Content-Type"), hdr.Get("Content-Disposition"), hdr.Get("X-Frame-Options"))
	}

	code, body, hdr = alice.fetchRaw(m.Attachments[1].URL + "?download=1")
	if code != http.StatusOK || string(body) != pdf || !strings.HasPrefix(hdr.Get("Content-Disposition"), "attachment") {
		t.Errorf("pdf part = %d %q", code, hdr.Get("Content-Disposition"))
	}

	if _, _, hdr = alice.fetchRaw(m.Attachments[2].URL); !strings.HasPrefix(hdr.Get("Content-Disposition"), "attachment") {
		t.Errorf("an html part was offered inline: %q", hdr.Get("Content-Disposition"))
	}

	for _, bad := range []string{"/mail/parts/9", "/mail/parts/x"} {
		if code, _, _ := alice.fetchRaw(up.File.URL + bad); code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", bad, code)
		}
	}

	// A file that is not a message is refused, a stranger sees nothing.
	var shot struct {
		File struct {
			URL string `json:"url"`
		} `json:"file"`
	}

	alice.upload(sketch, n.Node.ID, "shot.png", png, &shot)
	alice.expect(http.StatusBadRequest, "GET", shot.File.URL+"/mail", nil)
	bob.expect(http.StatusNotFound, "GET", up.File.URL+"/mail", nil)
	bob.expect(http.StatusNotFound, "GET", m.Attachments[0].URL, nil)

	// The served file itself may sit in a frame of the console.
	if _, _, hdr = alice.fetchRaw(shot.File.URL); hdr.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("an inline file carries %q, want SAMEORIGIN so the viewer can frame it", hdr.Get("X-Frame-Options"))
	}
}

// A file on a deleted entity comes back with the undo: its bytes are
// kept for files.retain_deleted and let go by the sweep after, unless
// a record names them again.
func TestADeletedFileComesBackWithTheUndo(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	var doc struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "domain", "label": "example.com"}, &doc)

	var up struct {
		File struct {
			ID   string `json:"id"`
			Hash string `json:"hash"`
		} `json:"file"`
	}

	if code := alice.upload(sketch, doc.Node.ID, "notes.txt", "kept for a while", &up); code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}

	alice.expect(http.StatusOK, "POST", sketch+"/nodes/delete", map[string]any{"ids": []string{doc.Node.ID}})

	// The record went with the node, the bytes stay, marked released.
	if ok, _ := rt.Blobs.Exists(t.Context(), up.File.Hash); !ok {
		t.Fatal("the bytes went with the delete")
	}

	if released, _ := rt.Store.Attachment.ReleasedBefore(t.Context(), time.Now().UTC().Add(time.Hour)); len(released) != 1 {
		t.Fatalf("released = %v, want the file's blob", released)
	}

	// The sweep leaves a blob younger than the retention alone.
	if n, err := attachment.Sweep(t.Context(), rt, time.Now().UTC().Add(-time.Hour)); n != 0 || err != nil {
		t.Fatalf("an early sweep deleted %d, %v", n, err)
	}

	alice.expect(http.StatusOK, "POST", sketch+"/undo", nil)

	var files struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/nodes/"+doc.Node.ID+"/files", nil, &files)
	if len(files.Files) != 1 || files.Files[0].ID != up.File.ID || files.Files[0].Name != "notes.txt" {
		t.Fatalf("files after the undo: %+v", files.Files)
	}

	alice.expect(http.StatusOK, "GET", sketch+"/files/"+up.File.ID, nil)

	// Named again, the blob survives a late sweep and the mark goes.
	if n, err := attachment.Sweep(t.Context(), rt, time.Now().UTC().Add(time.Hour)); n != 0 || err != nil {
		t.Fatalf("the sweep deleted a file in use: %d, %v", n, err)
	}

	if released, _ := rt.Store.Attachment.ReleasedBefore(t.Context(), time.Now().UTC().Add(time.Hour)); len(released) != 0 {
		t.Errorf("a blob in use is still marked released: %v", released)
	}

	// Deleted again through redo, the retention over, the bytes go.
	alice.expect(http.StatusOK, "POST", sketch+"/redo", nil)
	if n, err := attachment.Sweep(t.Context(), rt, time.Now().UTC().Add(time.Hour)); n != 1 || err != nil {
		t.Fatalf("the late sweep deleted %d, %v, want 1", n, err)
	}

	if ok, _ := rt.Blobs.Exists(t.Context(), up.File.Hash); ok {
		t.Error("the bytes outlived the retention")
	}
}

// A file's metadata is read at upload: it sits on the record and fills
// the entity's empty fields, a photo's camera and place here.
func TestAFileFillsItsEntityAtUpload(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	var photo struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "photo", "label": "Harbour", "properties": map[string]any{"camera": "mine"}}, &photo)

	// A JPEG whose Exif names the camera: IFD0 with Make at offset 26.
	tiff := []byte("II*\x00\x08\x00\x00\x00\x01\x00\x0f\x01\x02\x00\x06\x00\x00\x00\x1a\x00\x00\x00\x00\x00\x00\x00Canon\x00")
	payload := append([]byte("Exif\x00\x00"), tiff...)
	jpg := binary.BigEndian.AppendUint16([]byte{0xFF, 0xD8, 0xFF, 0xE1}, uint16(len(payload)+2)) //nolint:gosec // a fixed payload of a few dozen bytes
	jpg = append(jpg, payload...)
	jpg = append(jpg, 0xFF, 0xD9)

	var up struct {
		File struct {
			Metadata map[string]any `json:"metadata"`
		} `json:"file"`
	}

	if code := alice.upload(sketch, photo.Node.ID, "shot.jpg", string(jpg), &up); code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}

	if up.File.Metadata["camera_make"] != "Canon" {
		t.Errorf("metadata = %v", up.File.Metadata)
	}

	var g graphBody
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
	if len(g.Nodes) != 1 || g.Nodes[0].Properties["camera"] != "mine" {
		t.Errorf("the person's own camera was overwritten: %v", g.Nodes[0].Properties)
	}
}

// An import names its files' types, and a type the serve path would
// render inline is not taken from it: "image/SVG+xml" would have been
// an svg running as the site. The type comes from the name and the
// bytes, and the serve rule folds case and parameters.
func TestAnImportedFileTypeIsSniffed(t *testing.T) {
	srv, _ := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	svg := `<svg xmlns="http://www.w3.org/2000/svg"><script>fetch("/api/auth/me")</script></svg>`
	var result struct {
		FilesCreated int `json:"files_created"`
	}

	alice.expect(http.StatusOK, "POST", sketch+"/import", map[string]any{
		"nodes": []map[string]any{{"id": "n1", "type": "website", "label": "https://example.com"}},
		"files": []map[string]any{{"node": "n1", "name": "logo.svg", "content_type": "image/SVG+xml",
			"data": base64.StdEncoding.EncodeToString([]byte(svg))}},
	}, &result)
	if result.FilesCreated != 1 {
		t.Fatalf("import = %+v", result)
	}

	var graph struct {
		Nodes []idBody `json:"nodes"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)

	var list struct {
		Files []struct {
			ContentType string `json:"content_type"`
			URL         string `json:"url"`
		} `json:"files"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/nodes/"+graph.Nodes[0].ID+"/files", nil, &list)
	if len(list.Files) != 1 || list.Files[0].ContentType != "image/svg+xml" {
		t.Fatalf("files = %+v", list.Files)
	}

	code, _, h := alice.fetchRaw(list.Files[0].URL)
	if code != http.StatusOK || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") {
		t.Errorf("an svg served = %d %v, want a download", code, h)
	}
}

// The bytes a new record names are written again when a sweep took them
// between the upload's Put and its record.
func TestAFileKeepsItsBytes(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	var doc struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "domain", "label": "example.com"}, &doc)

	var up struct {
		File struct {
			ID   string `json:"id"`
			Hash string `json:"hash"`
		} `json:"file"`
	}

	if code := alice.upload(sketch, doc.Node.ID, "notes.txt", "still here", &up); code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}

	if err := rt.Blobs.Delete(t.Context(), up.File.Hash); err != nil {
		t.Fatal(err)
	}

	if err := attachment.KeepBytes(t.Context(), rt, up.File.Hash, []byte("still here")); err != nil {
		t.Fatal(err)
	}

	if ok, _ := rt.Blobs.Exists(t.Context(), up.File.Hash); !ok {
		t.Fatal("the bytes were not written back")
	}
}

// A file over a files.max_size lowered since its upload is left out of
// an export rather than written cut.
func TestAnExportLeavesOutAFileOverTheLimit(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	var doc struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "domain", "label": "example.com"}, &doc)
	if code := alice.upload(sketch, doc.Node.ID, "big.txt", strings.Repeat("x", 64), nil); code != http.StatusCreated {
		t.Fatalf("upload big = %d", code)
	}

	if code := alice.upload(sketch, doc.Node.ID, "small.txt", "tiny", nil); code != http.StatusCreated {
		t.Fatalf("upload small = %d", code)
	}

	rt.FileLimits.Set(env.FileLimits{MaxSize: 16, RetainDeleted: time.Hour})

	var export struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}

	alice.expect(http.StatusOK, "POST", sketch+"/export", map[string]any{}, &export)
	if len(export.Files) != 1 || export.Files[0].Name != "small.txt" {
		t.Errorf("exported files = %+v, want small.txt alone", export.Files)
	}
}
