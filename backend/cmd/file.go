package main

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

var TmpRoot = filepath.Join(StorageRoot, "_tmp")

func sanitizePath(p string) string {
	c := filepath.Clean("/" + p) 
	if c == "/" {
		return ""
	}
	return c[1:] 
}

func uploadFile(db *sql.DB,w http.ResponseWriter, r *http.Request)  {
	// Authenticate auth token
	userId, errAuth := authenticateUser(db, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return 
	}

	// Find folder in db
	virtualPath := sanitizePath(r.FormValue("path"))
	var folderId int
	db.QueryRow(`SELECT id FROM folders WHERE owner_id=? AND name=?`,userId,virtualPath).Scan(&folderId)

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
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM tmp_uuid WHERE uuid=?)", uuid).Scan(&uuidExistsTmp)
		if uuidExists || uuidExistsTmp {
			continue
		}

		// create token if the token is unique
		_, err := db.Exec(`INSERT INTO tmp_uuid (uuid) VALUES (?)`,uuid)
		if err != nil {
			http.Error(w, "create tmp UUID failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		break
	}

	// Get header values
	ext := filepath.Ext(header.Filename)
	storedName := uuid + ext

	// Ensure tmp dir exists
	_ = os.MkdirAll(TmpRoot, 0755)

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

	// Register file
	if _, err := db.Exec(`DELETE FROM tmp_uuid WHERE uuid=?`,uuid); err!=nil{
		http.Error(w, "deletion of tmp uuid failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err:= db.Exec(`INSERT INTO files (uuid, owner_id, folder_id, stored_name, display_name, mime, size_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)`,uuid, userId, folderId, storedName, header.Filename, ext, written); err!=nil{
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
