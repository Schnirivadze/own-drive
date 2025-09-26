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
