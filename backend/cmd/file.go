package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type UploadReq struct {
	Path       string `json:"path"`
	Filename   string `json:"filename"`
	Mime       string `json:"mime"`
	Size_bytes int64  `json:"size_bytes"`
	Sha256     string `json:"sha256"`
}

var TmpRoot = filepath.Join(StorageRoot, "_tmp")

func sanitizePath(p string) string {
	c := filepath.Clean("/" + p)
	if c == "/" {
		return ""
	}
	return c[1:]
}

func uploadFile(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// Authenticate auth token
	userId, errAuth := authenticateUser(db, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Find folder in db
	virtualPath := sanitizePath(r.FormValue("path"))
	var folderId int
	db.QueryRow(`SELECT id FROM folders WHERE owner_id=? AND name=?`, userId, virtualPath).Scan(&folderId)

	// Parce form (32 MB)
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		http.Error(w, "failed to parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get file and header from form
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Generate uuid
	var uuid string
	for {
		// generate token
		uuid = generateRawToken()

		// check if token already exists
		var uuidExists bool
		var uuidExistsTmp bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=?)", uuid).Scan(&uuidExists)
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM uploads WHERE uuid=?)", uuid).Scan(&uuidExistsTmp)
		if uuidExists || uuidExistsTmp {
			continue
		}

		// create token if the token is unique
		_, err := db.Exec(`INSERT INTO uploads (uuid, owner_id, folder_id, stored_name, display_name) VALUES (?, ?, ?, ?, ?)`, uuid, userId, folderId, uuid, header.Filename, "not set", -1)
		if err != nil {
			http.Error(w, "create upload failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		break
	}

	// Get header values
	ext := filepath.Ext(header.Filename)
	storedName := uuid
	sha256 := strings.ToLower(r.FormValue("sha256"))

	// Write to temp file
	tmpPath := filepath.Join(TmpRoot, uuid+".part")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "create tmp file failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	written, err := io.Copy(tmpFile, file)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		http.Error(w, "sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	// Check hash
	if fileSha, err := calculateFileSha256(tmpPath); sha256 != fileSha || err != nil {
		if err != nil {
			http.Error(w, "error geting sha of a file: "+err.Error(), http.StatusInternalServerError)
			return
		} else {
			http.Error(w, "hashes of files do not match", http.StatusForbidden)
			return

		}
	}

	// Register file
	if _, err := db.Exec(`DELETE FROM uploads WHERE uuid=?`, uuid); err != nil {
		http.Error(w, "deletion of tmp uuid failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := db.Exec(`INSERT INTO files (uuid, owner_id, folder_id, stored_name, display_name, mime, size_bytes, sha256) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uuid, userId, folderId, storedName, header.Filename, ext, written, strings.ToLower(sha256)); err != nil {
		http.Error(w, "registration of file failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Move to files folder
	finalPath := filepath.Join(StorageRoot, storedName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		http.Error(w, "rename failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Success response
	w.WriteHeader(http.StatusOK)
}

func startUpload(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// Authenticate auth token
	userId, errAuth := authenticateUser(db, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized: "+errAuth.Error(), http.StatusUnauthorized)
		return
	}

	// Get upload data
	var upload UploadReq
	if err := json.NewDecoder(r.Body).Decode(&upload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Process data
	var folderId int
	if err := db.QueryRow(`SELECT id FROM folders WHERE owner_id=? AND name=?`, userId, upload.Path).Scan(&folderId); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Generate uuid
	var uuid string
	for {
		// generate token
		uuid = generateRawToken()

		// check if token already exists
		var uuidExists bool
		var uuidExistsTmp bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=?)", uuid).Scan(&uuidExists)
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM uploads WHERE uuid=?)", uuid).Scan(&uuidExistsTmp)
		if uuidExists || uuidExistsTmp {
			continue
		}

		break
	}

	// Create empty temp file
	tmpPath := filepath.Join(TmpRoot, uuid+".part")
	f, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "create tmp failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	f.Close()

	// Register upload
	if _, err := db.Exec(`INSERT INTO uploads (uuid, owner_id, folder_id, stored_name, display_name, mime, size_bytes, sha256, started_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid, userId, folderId, tmpPath, upload.Filename, upload.Mime, upload.Size_bytes, strings.ToLower(upload.Sha256), time.Now().UTC()); err != nil {
		http.Error(w, "registration of upload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond with uuid
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"upload_id":"%s"}`, uuid)
}

func uploadChunk(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	uuid := r.Header.Get("Authorization")
	var uuidExists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM uploads WHERE uuid=?)", uuid).Scan(&uuidExists)
	if !uuidExists {
		http.Error(w, "Invalid UUID", http.StatusInternalServerError)
		return
	}

	offsetStr := r.URL.Query().Get("offset")
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)
	tmpPath := filepath.Join(TmpRoot, uuid+".part")

	// Open file
	f, err := os.OpenFile(tmpPath, os.O_WRONLY, 0644)
	if err != nil {
		http.Error(w, "open tmp failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// Seek to offset then write
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		http.Error(w, "seek failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy body
	if _, err := io.Copy(f, r.Body); err != nil {
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := f.Sync(); err != nil {
		http.Error(w, "sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func finishUpload(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	uuid := r.Header.Get("Authorization")
	var uuidExists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM uploads WHERE uuid=?)", uuid).Scan(&uuidExists)
	if !uuidExists {
		http.Error(w, "Invalid UUID", http.StatusInternalServerError)
		return
	}

	// Get data from uploads
	var owner_id, folder_id, stored_name, display_name, mime, sha256 string
	var size_bytes int
	if err := db.QueryRow(`SELECT owner_id, folder_id, stored_name, display_name, mime, size_bytes, sha256 FROM uploads WHERE uuid=? `, uuid).Scan(&owner_id, &folder_id, &stored_name, &display_name, &mime, &size_bytes, &sha256); err != nil {
		http.Error(w, "Couldnt get metadata of upload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check hash
	tmpPath := filepath.Join(TmpRoot, uuid+".part")
	finalPath := filepath.Join(StorageRoot, uuid)
	if fileSha, err := calculateFileSha256(tmpPath); sha256 != fileSha || err != nil {
		if err != nil {
			http.Error(w, "error geting sha of a file: "+err.Error(), http.StatusInternalServerError)
			return
		} else {
			http.Error(w, "hashes of files do not match", http.StatusForbidden)
			return

		}
	}

	// Register file
	if _, err := db.Exec(`DELETE FROM uploads WHERE uuid=?`, uuid); err != nil {
		http.Error(w, "deletion of tmp uuid failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := db.Exec(`INSERT INTO files (uuid, owner_id, folder_id, stored_name, display_name, mime, size_bytes, sha256) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uuid, owner_id, folder_id, finalPath, display_name, mime, size_bytes, sha256); err != nil {
		http.Error(w, "registration of file failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Move temp file to root folder
	if err := os.Rename(tmpPath, finalPath); err != nil {
		http.Error(w, "rename failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func downloadFileByUUID(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// Authenticate user
	authToken := r.URL.Query().Get("auth")
	userID, err := authenticateUser(db, authToken)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get uuid from url
	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		http.Error(w, "missing uuid", http.StatusBadRequest)
		return
	}

	// Get file metadata
	var ownerID int
	var storedName, displayName, mime, sha256 string
	var sizeBytes int64
	row := db.QueryRow(`SELECT owner_id, stored_name, display_name, mime, size_bytes, sha256 FROM files WHERE uuid = ?`, uuid)
	if err := row.Scan(&ownerID, &storedName, &displayName, &mime, &sizeBytes, &sha256); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "No such file", http.StatusNotFound)
			return
		}
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission
	if ownerID != userID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Construct full path and check file exists
	fi, err := os.Stat(storedName)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No such file: %s", storedName)
			http.Error(w, "No such file", http.StatusNotFound)
			return
		}
		http.Error(w, "stat error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Set headers
	if mime != "" {
		w.Header().Set("Content-Type", mime)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	// Suggest filename for browser download; sanitize displayName if needed
	safeName := filepath.Base(displayName)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(safeName)))

	// Serve file
	f, err := os.Open(storedName)
	if err != nil {
		http.Error(w, "open error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	// http.ServeContent uses the provided modtime; we can use fi.ModTime()
	http.ServeContent(w, r, safeName, fi.ModTime(), f)
}
