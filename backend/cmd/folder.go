package main

import (
	"database/sql"
	"errors"
	"log"
	"strings"
)

func getFolderIdFromPath(db *sql.DB, path string, ownerId int) (int, error) {
	// Check the path
	if path == "" || path[0] != '~' {
		return -1, errors.New("invalid path")
	}

	// Split the path into folders
	folders := strings.Split(path, "/")

	// Get users root folder
	var rootFolderId int
	if err := db.QueryRow(`SELECT id FROM folders WHERE owner_id=? AND name=?`, ownerId, "~").Scan(&rootFolderId); err != nil {
		return -1, err
	}

	// Follow folder branch
	var folderId = rootFolderId
	var folderName string
	for i := 1; i < len(folders); i++ {
		folderName = folders[i]
		if err := db.QueryRow(`SELECT id FROM folders WHERE owner_id=? AND name=? AND parent_id=?`, ownerId, folderName, folderId).Scan(&folderId); err != nil {
			return -1, errors.New("invalid path")
		}
	}

	return folderId, nil
}

func createFolder(db *sql.DB, folderPath string, ownerId int) error {
	// Get paths
	lastSlashIndex := strings.LastIndex(folderPath, "/")
	folderName := folderPath[lastSlashIndex+1:]
	parentFolderPath := folderPath[:lastSlashIndex-1]

	// Get parent folder id
	parentFolderId, errFolderId := getFolderIdFromPath(db, parentFolderPath, ownerId)
	if errFolderId != nil {
		return errFolderId
	}

	// Create folder in db
	_, errCreateFolder := db.Exec(`INSERT INTO folders (owner_id, name, parent_id)`, ownerId, folderName, parentFolderId)
	return errCreateFolder
}

func renameFolder(db *sql.DB, folderPath, name string, ownerId int) error {
	// Get folder id
	folderId, errFolderId := getFolderIdFromPath(db, folderPath, ownerId)
	if errFolderId != nil {
		return errFolderId
	}

	// Rename folder in db
	_, errDeleteFolder := db.Exec(`UPDATE folders SET name=? WHERE id=?`, name, folderId)
	return errDeleteFolder
}

func deleteFolder(db *sql.DB, folderPath string, ownerId int) error {
	// Get folder id
	folderId, errFolderId := getFolderIdFromPath(db, folderPath, ownerId)
	if errFolderId != nil {
		return errFolderId
	}

	// Delete folders and contens recursicely (rm -r)
	errDeleteFolderContents := deleteFolderContents(db, folderId, ownerId)
	return errDeleteFolderContents
}

func deleteFolderContents(db *sql.DB, folderId, ownerId int) error {
	// Get files in the folder
	fileRows, errFilesQuery := db.Query(`SELECT uuid FROM files WHERE folder_id=?`, folderId)
	if errFilesQuery != nil {
		return errFilesQuery
	}
	defer fileRows.Close()

	// Delete files in the folder
	for fileRows.Next() {
		var uuid string
		if err := fileRows.Scan(&uuid); err != nil {
			return err
		}
		if err := deleteFile(db, ownerId, uuid); err != nil {
			return err
		}
	}

	// Get child folders
	folderRows, errFolderQuery := db.Query(`SELECT folder_id FROM folders WHERE parent_id=?`, folderId)
	if errFolderQuery != nil {
		return errFolderQuery
	}
	defer folderRows.Close()

	// Remove folders recursively
	for folderRows.Next() {
		var folderId int
		if err := folderRows.Scan(&folderId); err != nil {
			return err
		}
		if err := deleteFolderContents(db, folderId, ownerId); err != nil {
			return err
		}
	}

	// Remove the folder
	if _, err := db.Exec(`DELETE FROM folders WHERE id=?`, folderId); err != nil {
		return err
	}

	return nil
}

func listFolderContents(db *sql.DB, folderPath string, ownerId int) (FolderContents, error) {
	var contents FolderContents

	// Get folder id
	folderId, errFolderId := getFolderIdFromPath(DB, folderPath, ownerId)
	if errFolderId != nil {
		log.Printf("Couldnt get folder id: %s", errFolderId.Error())
		return contents, errFolderId
	}

	// Get files in the folder
	fileRows, errFileQuery := db.Query(`SELECT uuid, display_name, mime, size_bytes, sha256, created_at FROM files WHERE owner_id = ? AND folder_id = ? AND deleted_at IS NULL`, ownerId, folderId)

	if errFileQuery != nil && errFileQuery != sql.ErrNoRows {
		log.Printf("Couldnt get file rows: %s", errFileQuery.Error())
		return contents, errFileQuery
	}
	defer fileRows.Close()

	// Turn sql rows into structs
	files := make([]FileWrapper, 0)
	for fileRows.Next() {
		var f FileWrapper
		var (
			dbMime sql.NullString
		)

		if err := fileRows.Scan(&f.UUID, &f.DisplayName, &dbMime, &f.SizeBytes, &f.Sha256, &f.CreatedAt); err != nil {
			log.Printf("Couldnt scan file rows: %s", err.Error())
			return contents, err
		}

		if dbMime.Valid {
			f.Mime = &dbMime.String
		}

		files = append(files, f)
	}
	if err := fileRows.Err(); err != nil {
		log.Printf("Couldnt iterate file rows: %s", err.Error())
		return contents, err
	}

	// Get files in the folder
	folderRows, errFolderQuery := db.Query(`SELECT name, created_at FROM folders WHERE owner_id = ? AND parent_id = ?`, ownerId, folderId)

	if errFolderQuery != nil && errFolderQuery != sql.ErrNoRows {
		log.Printf("Couldnt get folder rows: %s", errFolderQuery.Error())
		return contents, errFolderQuery
	}
	defer fileRows.Close()

	// Turn sql rows into structs
	folders := make([]FolderWrapper, 0)
	for folderRows.Next() {
		var f FolderWrapper
		if err := fileRows.Scan(&f.Name, &f.CreatedAt); err != nil {
			log.Printf("Couldnt scan folder rows: %s", err.Error())
			return contents, err
		}
		folders = append(folders, f)
	}
	if err := folderRows.Err(); err != nil {
		log.Printf("Couldnt iterate folder rows: %s", err.Error())
		return contents, err
	}

	contents.Folders = folders
	contents.Files = files
	return contents, nil
}
