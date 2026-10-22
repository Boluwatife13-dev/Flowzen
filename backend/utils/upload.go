package utils

import (
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowzen/database"
	"flowzen/models"
)

var FileTypes = []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".csv", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".zip"}
var ImageTypes = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

// SaveUpload checks an uploaded file's type, size and content, then stores it under a random name.
func SaveUpload(r *http.Request, field string, maxSize int64, allowed []string, folder string) (models.File, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxSize+10240)
	upload, header, err := r.FormFile(field)
	if err != nil {
		return models.File{}, Fail(400, "no file received, or the file is too large")
	}
	defer upload.Close()
	name := filepath.Base(header.Filename)
	ext := strings.ToLower(filepath.Ext(name))
	if !Contains(allowed, ext) {
		return models.File{}, Fail(400, "files of type "+ext+" are not allowed")
	}
	head := make([]byte, 512) // look at the first bytes to block disguised files
	n, _ := upload.Read(head)
	kind := http.DetectContentType(head[:n])
	if strings.HasPrefix(kind, "text/html") || (Contains(ImageTypes, ext) && !strings.HasPrefix(kind, "image/")) {
		return models.File{}, Fail(400, "the file content does not match its type")
	}
	upload.Seek(0, io.SeekStart)
	path := filepath.Join(folder, RandomHex(12)+ext)
	out, err := os.Create(path)
	if err != nil {
		return models.File{}, err
	}
	size, err := io.Copy(out, upload)
	out.Close()
	if err != nil {
		os.Remove(path)
		return models.File{}, err
	}
	fileType := mime.TypeByExtension(ext)
	if fileType == "" {
		fileType = "application/octet-stream"
	}
	return models.File{ID: RandomHex(6), Name: name, Type: fileType, Size: size, Path: path, UploadedAt: time.Now()}, nil
}

func RemoveFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		database.Log("error", "could not delete file: "+err.Error())
	}
}
