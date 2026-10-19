package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// DocInfo is the document record shared with the frontend.
type DocInfo struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Path         string  `json:"path"`
	PageCount    int     `json:"pageCount"`
	LastPage     int     `json:"lastPage"`
	Offset       float64 `json:"offset"`
	Zoom         float64 `json:"zoom"`
	FurthestPage int     `json:"furthestPage"`
	UpdatedAt    int64   `json:"updatedAt"`
	Exists       bool    `json:"exists"`
}

// ReadingState is persisted separately from the annotation payload so a
// position update never has to rewrite the stroke data.
type ReadingState struct {
	DocID        string  `json:"docId"`
	Page         int     `json:"page"`
	Offset       float64 `json:"offset"`
	Zoom         float64 `json:"zoom"`
	FurthestPage int     `json:"furthestPage"`
	UpdatedAt    int64   `json:"updatedAt"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Stroke struct {
	ID        string  `json:"id"`
	Page      int     `json:"page"`
	Tool      string  `json:"tool"`
	Color     string  `json:"color"`
	Width     float64 `json:"width"`
	Points    []Point `json:"points"`
	CreatedAt int64   `json:"createdAt"`
}

type Bookmark struct {
	ID        string `json:"id"`
	Page      int    `json:"page"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"createdAt"`
}

type TextNote struct {
	ID        string  `json:"id"`
	Page      int     `json:"page"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Size      float64 `json:"size"`
	Color     string  `json:"color"`
	Text      string  `json:"text"`
	CreatedAt int64   `json:"createdAt"`
}

type notesFile struct {
	Version int      `json:"version"`
	Strokes []Stroke `json:"strokes"`
}

type bookmarksFile struct {
	Version   int        `json:"version"`
	Bookmarks []Bookmark `json:"bookmarks"`
}

type textsFile struct {
	Version int        `json:"version"`
	Texts   []TextNote `json:"texts"`
}

type libraryState struct {
	LastDocID string             `json:"lastDocId"`
	Docs      map[string]DocInfo `json:"docs"`
}

// App owns the local library and the loopback PDF server.
type App struct {
	ctx context.Context

	mu           sync.Mutex
	dataDir      string
	notesDir     string
	bookmarksDir string
	textsDir     string
	libraryPath  string
	library      libraryState

	pdfSrv     *pdfServer
	pendingDoc *DocInfo
}

func NewApp() *App {
	return &App{
		library: libraryState{Docs: map[string]DocInfo{}},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = "."
	}
	a.dataDir = filepath.Join(configDir, "SimpleReader")
	a.notesDir = filepath.Join(a.dataDir, "notes")
	a.bookmarksDir = filepath.Join(a.dataDir, "bookmarks")
	a.textsDir = filepath.Join(a.dataDir, "texts")
	a.libraryPath = filepath.Join(a.dataDir, "library.json")

	if err := os.MkdirAll(a.notesDir, 0o755); err != nil {
		fmt.Println("create notes directory:", err)
	}
	if err := os.MkdirAll(a.bookmarksDir, 0o755); err != nil {
		fmt.Println("create data directory:", err)
	}
	if err := os.MkdirAll(a.textsDir, 0o755); err != nil {
		fmt.Println("create text directory:", err)
	}

	if data, err := os.ReadFile(a.libraryPath); err == nil {
		var lib libraryState
		if json.Unmarshal(data, &lib) == nil {
			a.library = lib
		}
	}
	if a.library.Docs == nil {
		a.library.Docs = map[string]DocInfo{}
	}

	a.pdfSrv, _ = newPDFServer()
	if a.pdfSrv != nil {
		if err := a.pdfSrv.start(); err != nil {
			fmt.Println("start pdf server:", err)
			a.pdfSrv = nil
		}
	}

	wruntime.OnFileDrop(ctx, func(_, _ int, paths []string) {
		if len(paths) > 0 {
			wruntime.EventsEmit(ctx, "file-drop", paths)
		}
	})

	for _, arg := range os.Args[1:] {
		if !strings.EqualFold(filepath.Ext(arg), ".pdf") {
			continue
		}
		if doc, err := a.OpenPath(arg); err == nil && doc.ID != "" {
			a.mu.Lock()
			a.pendingDoc = &doc
			a.mu.Unlock()
		}
		break
	}
}

func (a *App) shutdown(_ context.Context) {
	if a.pdfSrv != nil {
		a.pdfSrv.close()
	}
}

// OpenPDF shows the native file picker and opens the selected document.
func (a *App) OpenPDF() (DocInfo, error) {
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "打开 PDF",
		Filters: []wruntime.FileFilter{
			{DisplayName: "PDF 文件", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return DocInfo{}, err
	}
	if strings.TrimSpace(path) == "" {
		return DocInfo{}, nil
	}
	return a.OpenPath(path)
}

// OpenPath opens a document by absolute path. It is also used by drag/drop.
func (a *App) OpenPath(path string) (DocInfo, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return DocInfo{}, err
	}
	absPath = filepath.Clean(absPath)

	info, err := os.Stat(absPath)
	if err != nil {
		return DocInfo{}, fmt.Errorf("无法读取文件：%w", err)
	}
	if info.IsDir() {
		return DocInfo{}, errors.New("请选择一个 PDF 文件")
	}
	if err := validatePDF(absPath); err != nil {
		return DocInfo{}, err
	}

	id, err := hashFile(absPath)
	if err != nil {
		return DocInfo{}, fmt.Errorf("读取文件失败：%w", err)
	}

	a.mu.Lock()
	doc, exists := a.library.Docs[id]
	if !exists {
		doc = DocInfo{
			ID:           id,
			LastPage:     1,
			Zoom:         1,
			FurthestPage: 1,
		}
	}
	doc.Name = filepath.Base(absPath)
	doc.Path = absPath
	doc.Exists = true
	doc.UpdatedAt = time.Now().UnixMilli()
	if doc.LastPage < 1 {
		doc.LastPage = 1
	}
	if doc.Zoom <= 0 {
		doc.Zoom = 1
	}
	a.library.Docs[id] = doc
	a.library.LastDocID = id
	err = a.saveLibraryLocked()
	a.mu.Unlock()
	if err != nil {
		return DocInfo{}, err
	}

	if a.pdfSrv != nil {
		a.pdfSrv.register(id, absPath)
	}
	return doc, nil
}

// OpenRecent reopens a document from the recent list.
func (a *App) OpenRecent(docID string) (DocInfo, error) {
	a.mu.Lock()
	doc, ok := a.library.Docs[docID]
	a.mu.Unlock()
	if !ok {
		return DocInfo{}, errors.New("找不到这份文档")
	}
	if _, err := os.Stat(doc.Path); err != nil {
		return DocInfo{}, fmt.Errorf("文件不存在或已移动：%s", doc.Path)
	}
	return a.OpenPath(doc.Path)
}

func (a *App) GetRecentDocs() []DocInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	docs := make([]DocInfo, 0, len(a.library.Docs))
	for _, doc := range a.library.Docs {
		_, err := os.Stat(doc.Path)
		doc.Exists = err == nil
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].UpdatedAt > docs[j].UpdatedAt
	})
	if len(docs) > 20 {
		docs = docs[:20]
	}
	return docs
}

func (a *App) RemoveRecent(docID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.library.Docs, docID)
	if a.library.LastDocID == docID {
		a.library.LastDocID = ""
	}
	return a.saveLibraryLocked()
}

func (a *App) GetReadingState(docID string) ReadingState {
	a.mu.Lock()
	defer a.mu.Unlock()

	doc, ok := a.library.Docs[docID]
	if !ok {
		return ReadingState{DocID: docID, Page: 1, Zoom: 1, FurthestPage: 1}
	}
	page := doc.LastPage
	if page < 1 {
		page = 1
	}
	furthest := doc.FurthestPage
	if furthest < page {
		furthest = page
	}
	return ReadingState{
		DocID:        docID,
		Page:         page,
		Offset:       clamp01(doc.Offset),
		Zoom:         clampZoom(doc.Zoom),
		FurthestPage: furthest,
		UpdatedAt:    doc.UpdatedAt,
	}
}

func (a *App) SaveReadingState(state ReadingState) error {
	if state.DocID == "" {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	doc, ok := a.library.Docs[state.DocID]
	if !ok {
		return errors.New("文档不在本地记录中")
	}
	page := state.Page
	if page < 1 {
		page = 1
	}
	if page > doc.PageCount && doc.PageCount > 0 {
		page = doc.PageCount
	}
	furthest := state.FurthestPage
	if furthest < page {
		furthest = page
	}
	if furthest < doc.FurthestPage {
		furthest = doc.FurthestPage
	}

	doc.LastPage = page
	doc.Offset = clamp01(state.Offset)
	doc.Zoom = clampZoom(state.Zoom)
	doc.FurthestPage = furthest
	doc.UpdatedAt = time.Now().UnixMilli()
	a.library.Docs[state.DocID] = doc
	a.library.LastDocID = state.DocID
	return a.saveLibraryLocked()
}

func (a *App) SetPageCount(docID string, pageCount int) error {
	if pageCount < 1 {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	doc, ok := a.library.Docs[docID]
	if !ok {
		return errors.New("文档不在本地记录中")
	}
	if doc.PageCount == pageCount {
		return nil
	}
	doc.PageCount = pageCount
	if doc.LastPage > pageCount {
		doc.LastPage = pageCount
	}
	a.library.Docs[docID] = doc
	return a.saveLibraryLocked()
}

func (a *App) LoadStrokes(docID string) ([]Stroke, error) {
	data, err := os.ReadFile(filepath.Join(a.notesDir, docID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return []Stroke{}, nil
	}
	if err != nil {
		return nil, err
	}
	var notes notesFile
	if err := json.Unmarshal(data, &notes); err != nil {
		return nil, fmt.Errorf("批注文件损坏：%w", err)
	}
	if notes.Strokes == nil {
		return []Stroke{}, nil
	}
	return notes.Strokes, nil
}

func (a *App) SaveStrokes(docID string, strokes []Stroke) error {
	if docID == "" {
		return errors.New("缺少文档 ID")
	}
	if strokes == nil {
		strokes = []Stroke{}
	}
	data, err := json.Marshal(notesFile{Version: 1, Strokes: strokes})
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(a.notesDir, docID+".json"), data)
}

func (a *App) LoadBookmarks(docID string) ([]Bookmark, error) {
	data, err := os.ReadFile(filepath.Join(a.bookmarksDir, docID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return []Bookmark{}, nil
	}
	if err != nil {
		return nil, err
	}
	var saved bookmarksFile
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("书签文件损坏：%w", err)
	}
	if saved.Bookmarks == nil {
		return []Bookmark{}, nil
	}
	return saved.Bookmarks, nil
}

func (a *App) SaveBookmarks(docID string, bookmarks []Bookmark) error {
	if docID == "" {
		return errors.New("缺少文档 ID")
	}
	if bookmarks == nil {
		bookmarks = []Bookmark{}
	}
	data, err := json.Marshal(bookmarksFile{Version: 1, Bookmarks: bookmarks})
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(a.bookmarksDir, docID+".json"), data)
}

func (a *App) LoadTexts(docID string) ([]TextNote, error) {
	data, err := os.ReadFile(filepath.Join(a.textsDir, docID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return []TextNote{}, nil
	}
	if err != nil {
		return nil, err
	}
	var saved textsFile
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("文字批注文件损坏：%w", err)
	}
	if saved.Texts == nil {
		return []TextNote{}, nil
	}
	return saved.Texts, nil
}

func (a *App) SaveTexts(docID string, texts []TextNote) error {
	if docID == "" {
		return errors.New("缺少文档 ID")
	}
	if texts == nil {
		texts = []TextNote{}
	}
	data, err := json.Marshal(textsFile{Version: 1, Texts: texts})
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(a.textsDir, docID+".json"), data)
}

// ExportAnnotatedPDF asks for a destination and writes the PDF generated by
// the frontend. The sidecar JSON remains the editable source of annotations.
func (a *App) ExportAnnotatedPDF(suggestedName, contentBase64 string) (string, error) {
	defaultName := filepath.Base(strings.TrimSpace(suggestedName))
	if defaultName == "" || defaultName == "." {
		defaultName = "annotated.pdf"
	}
	if !strings.EqualFold(filepath.Ext(defaultName), ".pdf") {
		defaultName += ".pdf"
	}

	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "导出带批注 PDF",
		DefaultFilename: defaultName,
		Filters: []wruntime.FileFilter{
			{DisplayName: "PDF 文件", Pattern: "*.pdf"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return "", fmt.Errorf("导出数据无效：%w", err)
	}
	if err := atomicWrite(path, data); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) handleSecondInstance(args []string) {
	if a.ctx == nil {
		return
	}
	wruntime.WindowShow(a.ctx)
	wruntime.WindowUnminimise(a.ctx)
	for _, arg := range args {
		if strings.EqualFold(filepath.Ext(arg), ".pdf") {
			wruntime.EventsEmit(a.ctx, "file-drop", []string{arg})
			return
		}
	}
}

func (a *App) GetPDFURL(docID string) (string, error) {
	if a.pdfSrv == nil {
		return "", errors.New("本地 PDF 服务未启动")
	}

	a.mu.Lock()
	doc, ok := a.library.Docs[docID]
	a.mu.Unlock()
	if !ok {
		return "", errors.New("找不到这份文档")
	}
	if _, err := os.Stat(doc.Path); err != nil {
		return "", fmt.Errorf("文件不存在或已移动：%s", doc.Path)
	}
	a.pdfSrv.register(docID, doc.Path)
	return a.pdfSrv.url(docID), nil
}

// TakeStartupDoc returns a document passed on the command line, then clears it.
// This also makes file association / "open with" straightforward later.
func (a *App) TakeStartupDoc() (DocInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pendingDoc == nil {
		return DocInfo{}, nil
	}
	doc := *a.pendingDoc
	a.pendingDoc = nil
	return doc, nil
}

func (a *App) saveLibraryLocked() error {
	data, err := json.MarshalIndent(a.library, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(a.libraryPath, data)
}

func validatePDF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	header := make([]byte, 5)
	if _, err := io.ReadFull(f, header); err != nil {
		return errors.New("不是有效的 PDF 文件")
	}
	if string(header) != "%PDF-" {
		return errors.New("不是有效的 PDF 文件")
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampZoom(v float64) float64 {
	if v < 0.25 {
		return 0.25
	}
	if v > 5 {
		return 5
	}
	return v
}

// pdfServer is a tiny read-only loopback server. Keeping PDF bytes out of the
// JS bridge avoids base64 copies of large documents.
type pdfServer struct {
	mu      sync.RWMutex
	docs    map[string]string
	token   string
	baseURL string
	srv     *http.Server
}

func newPDFServer() (*pdfServer, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	return &pdfServer{
		docs:  map[string]string{},
		token: hex.EncodeToString(tokenBytes),
	}, nil
}

func (s *pdfServer) start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.baseURL = "http://" + listener.Addr().String()
	s.srv = &http.Server{Handler: s}
	go func() {
		if err := s.srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Println("pdf server:", err)
		}
	}()
	return nil
}

func (s *pdfServer) close() {
	if s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
}

func (s *pdfServer) register(docID, path string) {
	s.mu.Lock()
	s.docs[docID] = path
	s.mu.Unlock()
}

func (s *pdfServer) url(docID string) string {
	return fmt.Sprintf("%s/pdf/%s?token=%s", s.baseURL, docID, s.token)
}

func (s *pdfServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Accept, Origin, Content-Type")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.URL.Query().Get("token") != s.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	docID := strings.TrimPrefix(r.URL.Path, "/pdf/")
	if docID == "" || docID == r.URL.Path || strings.ContainsAny(docID, `/\`) {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	path, ok := s.docs[docID]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "cannot stat pdf", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}
