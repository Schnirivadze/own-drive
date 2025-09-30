package main

import (
	"database/sql"
	"errors"
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
	fileRows, errFilesQuery := DB.Query(`SELECT uuid FROM files WHERE folder_id=?`, folderId)
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
		if err := deleteFile(db, uuid, ownerId); err != nil {
			return err
		}
	}

	// Get child folders
	folderRows, errFolderQuery := DB.Query(`SELECT folder_id FROM folders WHERE parent_id=?`, folderId)
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
