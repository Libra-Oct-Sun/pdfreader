package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAtomicWriteReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := atomicWrite(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("second")); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("got %q, want %q", data, "second")
	}
}

func TestPDFServerRangeRequest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.pdf")
	content := []byte("%PDF-1.7\nsample content")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	server, err := newPDFServer()
	if err != nil {
		t.Fatal(err)
	}
	server.register("doc-1", path)

	request := httptest.NewRequest(
		http.MethodGet,
		"/pdf/doc-1?token="+server.token,
		nil,
	)
	request.Header.Set("Range", "bytes=0-4")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("got status %d, want %d", recorder.Code, http.StatusPartialContent)
	}
	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "%PDF-" {
		t.Fatalf("got range body %q", body)
	}
}

func TestPDFServerRejectsBadToken(t *testing.T) {
	server, err := newPDFServer()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/pdf/doc-1?token=wrong", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestStrokeRoundTrip(t *testing.T) {
	app := &App{notesDir: t.TempDir()}
	want := []Stroke{
		{
			ID:        "stroke-1",
			Page:      3,
			Tool:      "pen",
			Color:     "#1f2937",
			Width:     0.002,
			Points:    []Point{{X: 0.1, Y: 0.2}, {X: 0.2, Y: 0.3}},
			CreatedAt: 123,
		},
	}

	if err := app.SaveStrokes("doc-1", want); err != nil {
		t.Fatal(err)
	}
	got, err := app.LoadStrokes("doc-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
