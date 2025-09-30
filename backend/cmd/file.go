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

	// Reserve space in db
	if err := reserveQuota(db, userId, header.Size); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get header values
	sha256 := strings.ToLower(r.FormValue("sha256"))
	mime := r.FormValue("mime")
	var filePath string
	var fileTempPath string

	// Generate uuid
	var uuid string
	for {
		// generate token
		uuid = generateRawToken()

		// check if token already exists
		var uuidExists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=?)", uuid).Scan(&uuidExists)
		if uuidExists {
			continue
		}

		// create token if the token is unique
		filePath = filepath.Join(StorageRoot, uuid)
		fileTempPath = filepath.Join(filePath, ".part")
		if _, err := db.Exec(`INSERT INTO files (uuid, owner_id, folder_id, stored_name, display_name, mime, size_bytes, size_bytes_on_disk, sha256, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid, userId, folderId, fileTempPath, header.Filename, mime, header.Size, 0, sha256, time.Now().UTC()); err != nil {
			http.Error(w, "register file failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		break
	}

	// Write to temp file
	tmpFile, err := os.Create(fileTempPath)
	if err != nil {
		http.Error(w, "create tmp file failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, header.Size)
		return
	}
	written, err := io.Copy(tmpFile, file)
	if err != nil {
		tmpFile.Close()
		os.Remove(fileTempPath)
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, written)
		return
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(fileTempPath)
		http.Error(w, "sync failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, written)
		return
	}
	tmpFile.Close()

	// Check hash
	if fileSha, err := calculateFileSha256(fileTempPath); sha256 != fileSha || err != nil {
		if err != nil {
			http.Error(w, "error geting sha of a file: "+err.Error(), http.StatusInternalServerError)
			releaseQuota(db, userId, written)
			return
		} else {
			http.Error(w, "hashes of files do not match", http.StatusForbidden)
			releaseQuota(db, userId, written)
			return
		}
	}

	// Remove .part extention
	if err := os.Rename(fileTempPath, filePath); err != nil {
		os.Remove(fileTempPath)
		http.Error(w, "rename failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, written)
		return
	}

	// Register file
	if _, err := db.Exec(`UPDATE files SET size_bytes_on_disk = ?, stored_name = ? WHERE uuid = ?`, written, filePath, uuid); err != nil {
		http.Error(w, "registration of file failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, written)
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

	// Reserve space in db
	if err := reserveQuota(db, userId, upload.Size_bytes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		if uuidExists || uuidExistsTmp {
			continue
		}

		break
	}

	// Create empty temp file
	tmpPath := filepath.Join(StorageRoot, uuid+".part")
	f, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "create tmp failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	f.Close()

	// Register file
	if _, err := db.Exec(`INSERT INTO files (uuid, owner_id, folder_id, stored_name, display_name, mime, size_bytes, size_bytes_on_disk, sha256, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid, userId, folderId, tmpPath, upload.Filename, upload.Mime, upload.Size_bytes, 0, strings.ToLower(upload.Sha256), time.Now().UTC()); err != nil {
		http.Error(w, "registration of file failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond with uuid
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"upload_id":"%s"}`, uuid)
}

func uploadChunk(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	uuid := r.Header.Get("Authorization")
	var uuidExists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=?)", uuid).Scan(&uuidExists)
	if !uuidExists {
		http.Error(w, "Invalid UUID", http.StatusInternalServerError)
		return
	}

	// Check size
	var expected, onDisk int64
	db.QueryRow(`SELECT size_bytes, size_bytes_on_disk FROM files WHERE uuid = ?`, uuid).Scan(&expected, &onDisk)
	if expected == onDisk {
		http.Error(w, "file was uploaded completely", http.StatusInternalServerError)
		return
	}

	offsetStr := r.URL.Query().Get("offset")
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)
	tmpPath := filepath.Join(StorageRoot, uuid+".part")

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
	written, err := io.Copy(f, r.Body)
	if err != nil {
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := f.Sync(); err != nil {
		http.Error(w, "sync failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update db
	newSize := offset + written
	if _, err := db.Exec(`UPDATE files SET size_bytes_on_disk = ? WHERE uuid = ?`, newSize, uuid); err != nil {
		http.Error(w, "db update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func finishUpload(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// Check uuid
	uuid := r.URL.Query().Get("uuid")
	var uuidExists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=?)", uuid).Scan(&uuidExists)
	if !uuidExists {
		http.Error(w, "Invalid UUID", http.StatusInternalServerError)
		return
	}

	// Check ownership
	userId, errAuth := authenticateUser(db, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var userOwnsFile bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=? AND owner_id=?)", uuid, userId).Scan(&userOwnsFile)
	if !userOwnsFile {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check size
	var expected, onDisk int64
	db.QueryRow(`SELECT size_bytes, size_bytes_on_disk FROM files WHERE uuid = ?`, uuid).Scan(&expected, &onDisk)
	if expected != onDisk {
		http.Error(w, "file wasnt uploaded completely", http.StatusInternalServerError)
		return
	}

	// Get hash
	var sha256 string
	if err := db.QueryRow(`SELECT sha256 FROM files WHERE uuid=? `, uuid).Scan(&sha256); err != nil {
		http.Error(w, "Couldnt get hash of uploaded file: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, onDisk)
		return
	}

	// Check hash
	tmpPath := filepath.Join(StorageRoot, uuid+".part")
	finalPath := filepath.Join(StorageRoot, uuid)
	if fileSha, err := calculateFileSha256(tmpPath); sha256 != fileSha || err != nil {
		if err != nil {
			http.Error(w, "error geting hash of a file: "+err.Error(), http.StatusInternalServerError)
			return
		} else {
			http.Error(w, "hashes of files do not match", http.StatusForbidden)
			releaseQuota(db, userId, onDisk)
			return
		}
	}

	// Update path
	if _, err := db.Exec(`UPDATE files SET stored_name = ? WHERE uuid = ?`, finalPath, uuid); err != nil {
		http.Error(w, "db update failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, onDisk)
		return
	}

	// Move temp file to root folder
	if err := os.Rename(tmpPath, finalPath); err != nil {
		http.Error(w, "rename failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, onDisk)
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

func deleteFile(db *sql.DB, uuid string, userId int) error {
	// Get file size and path
	var fileSize int64
	var storedName string
	if err := db.QueryRow(`SELECT size_bytes_on_disk, stored_name FROM files WHERE uuid=?`, uuid).Scan(&fileSize, &storedName); err != nil {
		return err
	}

	// Remove file from db
	if _, err := db.Exec(`DELETE FROM files WHERE uuid=?`, uuid, userId); err != nil {
		return err
	}

	// Update users space usage
	if err := releaseQuota(db, userId, fileSize); err != nil {
		return err
	}

	// Remove file from disk
	if err := os.Remove(storedName); err != nil {
		return err
	}

	return nil
}
