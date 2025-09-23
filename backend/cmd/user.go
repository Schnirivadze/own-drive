package main

import (
	"database/sql"
	"log"
)

func getUserIdByLogin(db *sql.DB, username, password string) (int, error) {
	var userId int
	err := db.QueryRow("SELECT id FROM users WHERE username=? AND password=?", username, password).Scan(&userId)
	return userId, err
}

func createUser(db *sql.DB, inviteToken string, username, password string) error {
	// Authenticate invite token
	var validToken bool
	errToken := db.QueryRow("SELECT EXISTS(SELECT 1 FROM invite_tokens WHERE token=?)", inviteToken).Scan(&validToken)
	if errToken != nil {
		log.Println("Error authenticating invite token")
		return errToken
	}
	if !validToken {
		log.Println("Invalid invite token")
		return sql.ErrNoRows
	}

	// Create user
	result, errCreateUser := db.Exec(`INSERT INTO users (username, password, quota_bytes) VALUES (?, ?, ?)`, username, password, DefaultQuotaBytes)
	if errCreateUser != nil {
		log.Println("Couldnt create user")
		return errCreateUser
	}
	userId, errGetId := result.LastInsertId()
	if errGetId != nil {
		log.Println("Couldnt get users id")
		return errGetId
	}

	// Create root folder
	if _, errCreateFolder := db.Exec(`INSERT INTO folders (owner_id, name) values (?, ?)`, userId, "~"); errCreateFolder!=nil{
		log.Println("Couldnt create users root folder")
		return errCreateFolder
	}

	// Remove invite token
	err := deleteInviteToken(db, inviteToken)
	return err
}

func deleteUser(db *sql.DB, authToken string) error {
	// Authenticate auth token
	userId, errAuth := authenticateUser(db, authToken)
	if errAuth != nil {
		log.Println("Invalid auth token")
		return errAuth
	}

	// Delete user
	_, err := db.Exec(`DELETE FROM users WHERE id=?`, userId)
	return err
}

func updateUser(db *sql.DB, authToken, username, password string) error {
	// Authenticate auth token
	userId, errAuth := authenticateUser(db, authToken)
	if errAuth != nil {
		log.Println("Invalid auth token")
		return errAuth
	}

	// Update username
	var oldUsername string
	errUsername := db.QueryRow("SELECT username FROM users WHERE id=?", userId).Scan(&oldUsername)
	if errUsername != nil {
		log.Println("Couldnt get old username")
		return errUsername
	}
	if username != oldUsername {
		_, errUpdUsername := db.Exec(`UPDATE users SET username = ? WHERE id = ?`, username, userId)
		if errUpdUsername != nil {
			log.Println("Couldnt update username")
			return errUpdUsername
		}
	}

	// Update password
	var oldPassword string
	errPassword := db.QueryRow("SELECT password FROM users WHERE id=?", userId).Scan(&oldPassword)
	if errPassword != nil {
		log.Println("Couldnt get old password")
		return errPassword
	}
	if password != oldPassword {
		_, errUpdPassword := db.Exec(`UPDATE users SET password = ? WHERE id = ?`, password, userId)
		if errUpdPassword != nil {
			log.Println("Couldnt update password")
			return errUpdPassword
		}
	}

	// Deauth user
	err := deleteAuthToken(db, authToken)
	return err
}
