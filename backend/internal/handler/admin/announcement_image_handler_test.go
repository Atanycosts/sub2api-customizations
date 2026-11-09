package admin

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newAnnouncementImageTestHandler(t *testing.T) (*AnnouncementHandler, string) {
	t.Helper()
	dataDir := t.TempDir()
	handler := NewAnnouncementHandlerWithConfig(nil, &config.Config{
		Pricing: config.PricingConfig{DataDir: dataDir},
	})
	return handler, filepath.Join(dataDir, "announcement-images")
}

func announcementPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 22, G: 163, B: 74, A: 255})
	var body bytes.Buffer
	require.NoError(t, png.Encode(&body, img))
	return body.Bytes()
}

func announcementMultipartRequest(t *testing.T, filename string, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements/assets", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func announcementImageUpload(t *testing.T, handler *AnnouncementHandler, req *http.Request) (*httptest.ResponseRecorder, response.Response) {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	handler.UploadImage(ctx)
	var envelope response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope), rec.Body.String())
	return rec, envelope
}

func TestAnnouncementImageUploadAndPublicRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, imageDir := newAnnouncementImageTestHandler(t)
	data := announcementPNG(t, 120, 80)
	rec, envelope := announcementImageUpload(t, handler, announcementMultipartRequest(t, "wide.png", data))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var uploaded announcementImageUploadResponse
	raw, err := json.Marshal(envelope.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &uploaded))
	require.Equal(t, 120, uploaded.Width)
	require.Equal(t, 80, uploaded.Height)
	require.Regexp(t, `^/api/v1/announcements/assets/[a-f0-9]{32}\.png$`, uploaded.URL)

	filename := strings.TrimPrefix(uploaded.URL, announcementImagePublicPath)
	stored, err := os.ReadFile(filepath.Join(imageDir, filename))
	require.NoError(t, err)
	require.Equal(t, data, stored)

	// Recreate the handler to prove reads use the configured persistent data directory.
	reloaded := NewAnnouncementHandlerWithConfig(nil, &config.Config{
		Pricing: config.PricingConfig{DataDir: filepath.Dir(imageDir)},
	})
	readRecorder := httptest.NewRecorder()
	readContext, _ := gin.CreateTestContext(readRecorder)
	readContext.Request = httptest.NewRequest(http.MethodGet, uploaded.URL, nil)
	readContext.Params = gin.Params{{Key: "filename", Value: filename}}
	reloaded.ServeImage(readContext)

	require.Equal(t, http.StatusOK, readRecorder.Code)
	require.Equal(t, data, readRecorder.Body.Bytes())
	require.Equal(t, "image/png", readRecorder.Header().Get("Content-Type"))
	require.Equal(t, "public, max-age=31536000, immutable", readRecorder.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", readRecorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "default-src 'none'; sandbox", readRecorder.Header().Get("Content-Security-Policy"))
}

func TestAnnouncementImageUploadRejectsInvalidAndOversizedContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, imageDir := newAnnouncementImageTestHandler(t)
	validPNG := announcementPNG(t, 2, 2)
	cases := []struct {
		name     string
		filename string
		data     []byte
	}{
		{name: "plain text", filename: "note.png", data: []byte("not an image")},
		{name: "svg", filename: "icon.png", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		{name: "truncated png", filename: "broken.png", data: announcementPNG(t, 2, 2)[:20]},
		{name: "truncated pixel data", filename: "pixels.png", data: validPNG[:len(validPNG)-12]},
		{name: "too large", filename: "large.png", data: append(announcementPNG(t, 2, 2), make([]byte, announcementImageMaxBytes)...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := announcementImageUpload(t, handler, announcementMultipartRequest(t, tc.filename, tc.data))
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
	entries, err := os.ReadDir(imageDir)
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	require.Empty(t, entries)
}

func TestAnnouncementImageUploadRejectsOversizedDimensions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, imageDir := newAnnouncementImageTestHandler(t)
	data := announcementPNG(t, announcementImageMaxSide+1, 1)
	rec, _ := announcementImageUpload(t, handler, announcementMultipartRequest(t, "wide.png", data))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	entries, err := os.ReadDir(imageDir)
	if !os.IsNotExist(err) {
		require.NoError(t, err)
	}
	require.Empty(t, entries)
}

func TestAnnouncementImageReadRejectsUnsafeFilenamesAndMissingFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, imageDir := newAnnouncementImageTestHandler(t)
	require.NoError(t, os.MkdirAll(imageDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(imageDir, "note.txt"), []byte("no"), 0o640))

	for _, filename := range []string{
		"note.txt",
		"../note.txt",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA.png",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.png",
	} {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest(http.MethodGet, announcementImagePublicPath+filename, nil)
		ctx.Params = gin.Params{{Key: "filename", Value: filename}}
		handler.ServeImage(ctx)
		ctx.Writer.WriteHeaderNow()
		require.Equal(t, http.StatusNotFound, rec.Code, filename)
		require.Empty(t, rec.Body.String(), filename)
	}
}
