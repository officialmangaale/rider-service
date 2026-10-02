package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func uploadRequest(t *testing.T, name string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(body)
	_ = mw.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/upload", &buf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	// A nil S3 client is fine: rejected uploads must never reach S3.
	(&UploadHandler{bucket: "test"}).HandleUpload(c)
	return w
}

func TestUploadRejectsNonDocumentContent(t *testing.T) {
	for name, body := range map[string][]byte{
		"page.html": []byte("<html><script>alert(1)</script></html>"),
		"logo.svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"fake.jpg":  []byte("<?php echo 1; ?>"),
	} {
		if w := uploadRequest(t, name, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, w.Code)
		}
	}
}

func TestUploadRejectsEmptyFile(t *testing.T) {
	if w := uploadRequest(t, "empty.png", nil); w.Code != http.StatusBadRequest {
		t.Errorf("empty file: status = %d, want 400", w.Code)
	}
}
