package main

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func startUpload(db *sql.DB, userId int, upload UploadReq) (string, error) {
	// Process data
	var folderId int
	folderId, errGetFolder := getFolderIdFromPath(db, upload.Path, userId)
	if errGetFolder != nil {
		log.Println("Couldnt get folder id")
		return "", errGetFolder
	}

	// Reserve space in db
	if err := reserveQuota(db, userId, upload.Size_bytes); err != nil {
		log.Println("Couldnt reserve quota")
		return "", err
	}

	// Generate uuid
	var uuid string
	for {
		// Generate token
		uuid = generateRawToken()

		// Check if token already exists
		var uuidExists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=?)", uuid).Scan(&uuidExists)
		if uuidExists {
			continue
		}

		// Continue with registration if token is valid
		break
	}

	// Create empty temp file
	tmpPath := filepath.Join(StorageRoot, uuid+".part")
	f, err := os.Create(tmpPath)
	if err != nil {
		log.Printf("Create tmp failed: %s", err.Error())
		return "", err
	}
	f.Close()

	// Register file
	if _, err := db.Exec(`INSERT INTO files (uuid, owner_id, folder_id, stored_name, display_name, mime, size_bytes, size_bytes_on_disk, sha256, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid, userId, folderId, tmpPath, upload.Filename, upload.Mime, upload.Size_bytes, 0, strings.ToLower(upload.Sha256), time.Now().UTC()); err != nil {
		log.Printf("Registration of file failed: %s", err.Error())
		return "", err
	}

	// Return uuid
	return uuid, nil
}

func uploadChunk(db *sql.DB, uuid, offsetStr string, bytes io.ReadCloser) error {
	// Check size
	var expected, onDisk int64
	db.QueryRow(`SELECT size_bytes, size_bytes_on_disk FROM files WHERE uuid = ?`, uuid).Scan(&expected, &onDisk)
	if expected == onDisk {
		log.Println("File was uploaded completely")
		return errors.New("file was uploaded completely")
	}

	offset, _ := strconv.ParseInt(offsetStr, 10, 64)
	tmpPath := filepath.Join(StorageRoot, uuid+".part")

	// Open file
	f, err := os.OpenFile(tmpPath, os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("open tmp failed: %s", err.Error())
		return err
	}
	defer f.Close()

	// Seek to offset then write
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		log.Printf("seek failed: %s", err.Error())
		return err
	}

	// Copy body
	written, err := io.Copy(f, bytes)
	if err != nil {
		log.Printf("write failed: %s", err.Error())
		return err
	}
	if err := f.Sync(); err != nil {
		log.Printf("sync failed: %s", err.Error())
		return err
	}

	// Update db
	newSize := offset + written
	if _, err := db.Exec(`UPDATE files SET size_bytes_on_disk = ? WHERE uuid = ?`, newSize, uuid); err != nil {
		log.Printf("db update failed: %s", err.Error())
		return err
	}

	return nil
}

func finishUpload(db *sql.DB, uuid string, userId int) error {
	// Check size
	var expected, onDisk int64
	db.QueryRow(`SELECT size_bytes, size_bytes_on_disk FROM files WHERE uuid=?`, uuid).Scan(&expected, &onDisk)
	if expected != onDisk {
		log.Println("File wasnt uploaded completely")
		return errors.New("file wasnt uploaded completely")
	}

	// Get hash
	var sha256 string
	if err := db.QueryRow(`SELECT sha256 FROM files WHERE uuid=?`, uuid).Scan(&sha256); err != nil {
		log.Println("Couldnt get hash of uploaded file: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, onDisk)
		return err
	}

	// Check hash
	tmpPath := filepath.Join(StorageRoot, uuid+".part")
	finalPath := filepath.Join(StorageRoot, uuid)
	if fileSha, err := calculateFileSha256(tmpPath); sha256 != fileSha || err != nil {
		if err != nil {
			log.Println("error geting hash of a file: "+err.Error(), http.StatusInternalServerError)
			return err
		} else {
			log.Println("hashes of files do not match", http.StatusForbidden)
			releaseQuota(db, userId, onDisk)
			return err
		}
	}

	// Update path
	if _, err := db.Exec(`UPDATE files SET stored_name = ? WHERE uuid = ?`, finalPath, uuid); err != nil {
		log.Println("db update failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, onDisk)
		return err
	}

	// Move temp file to root folder
	if err := os.Rename(tmpPath, finalPath); err != nil {
		log.Println("rename failed: "+err.Error(), http.StatusInternalServerError)
		releaseQuota(db, userId, onDisk)
		return err
	}

	return nil
}

func getFileByUUID(db *sql.DB, userId int, uuid string) (File *os.File, Mime string, SafeName string, ModTime time.Time, Err error) {
	// Get file metadata
	var ownerId int
	var storedName, displayName, mime, sha256 string
	var sizeBytes int64
	row := db.QueryRow(`SELECT owner_id, stored_name, display_name, mime, size_bytes, sha256 FROM files WHERE uuid = ?`, uuid)
	if err := row.Scan(&ownerId, &storedName, &displayName, &mime, &sizeBytes, &sha256); err != nil {
		if err == sql.ErrNoRows {
			log.Printf("No such file")
			return nil, "", "", time.Time{}, err
		}
		log.Printf("DB error: "+err.Error(), http.StatusInternalServerError)
		return nil, "", "", time.Time{}, err
	}

	// Check permission
	if ownerId != userId {
		return nil, "", "", time.Time{}, errors.New("Forbidden")
	}

	// Construct full path and check file exists
	fi, err := os.Stat(storedName)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No such file: %s", storedName)
			return nil, "", "", time.Time{}, err
		}
		log.Printf("stat error: "+err.Error(), http.StatusInternalServerError)
		return nil, "", "", time.Time{}, err
	}

	// Suggest filename
	safeName := filepath.Base(displayName)

	// Serve file
	f, err := os.Open(storedName)
	if err != nil {
		log.Printf("Open error: %s", err.Error())
		return nil, "", "", time.Time{}, err
	}

	return f, mime, safeName, fi.ModTime(), nil
}

func deleteFile(db *sql.DB, userId int, uuid string) error {
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

func renameFile(db *sql.DB, uuid, name string) error {
	// Update filename
	if _, err := db.Exec(`UPDATE files SET display_name=? WHERE uuid=?`, name, uuid); err != nil {
		return err
	}

	return nil
}
