package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"time"
)

func generateRawToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func createInviteToken(db *sql.DB) (string, error) {
	var token string
	var err error
	for {
		// generate token
		token = generateRawToken()

		// check if token already exists
		var tokenExists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM invite_tokens WHERE token=?)", token).Scan(&tokenExists)
		if tokenExists {
			continue
		}

		// create token if the token is unique
		break
	}

	// set expiry date
	expiresAt := time.Now().Add(InviteTokenValidHours * time.Hour).UTC()

	// inster token in db
	_, err = db.Exec(`INSERT INTO invite_tokens (token, expires_at) VALUES (?, ?)`, token, expiresAt)
	if err != nil {
		log.Println("Could not insert invite token")
	}
	return token, err
}

func deleteInviteToken(db *sql.DB, inviteToken string) error{
	_, err :=db.Exec(`DELETE FROM invite_tokens WHERE token=?`,inviteToken)
	return err
}

func createAuthToken(db *sql.DB, userId int) (string, string, error) {
	var token string = ""
	var err error
	for {
		// generate token
		token = generateRawToken()

		// check if token already exists
		var tempUserId int
		err1 := db.QueryRow("SELECT user_id FROM auth_tokens WHERE token=?", token).Scan(&tempUserId)
		if err1 != sql.ErrNoRows {
			continue
		}

		// check if user already has a token
		var tempToken string
		var tempExpiryDate time.Time
		err2 := db.QueryRow("SELECT token, expires_at FROM auth_tokens WHERE user_id = ?", userId).Scan(&tempToken, &tempExpiryDate)
		if err2 == nil {
			// compare expiryDate with current time
			if time.Now().After(tempExpiryDate) {
				_, delErr := db.Exec("DELETE FROM auth_tokens WHERE user_id = ?", userId)
				if delErr != nil {
					log.Println("Could not delete auth token")
					return "", "", delErr
				}
				break
			}

			// return token that user has and which isnt exired
			return tempToken, tempExpiryDate.String(), nil

		}

		// break the loop if token is unique and user doesnt have any
		break
	}

	// set expiry date
	expiresAt := time.Now().Add(6 * time.Hour).UTC()

	// inster token in db
	_, err = db.Exec(`INSERT INTO auth_tokens (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userId, expiresAt)
	if err != nil {
		log.Println("Could not insert auth token")
		return "", "", err
	}

	return token, expiresAt.String(), nil
}

func deleteAuthToken(db *sql.DB, authToken string) error{
	_, err := db.Exec(`DELETE FROM auth_tokens WHERE token=?`,authToken)
	return err
}

func authenticateAdmin(db *sql.DB, authToken string) (bool, error) {
	// check if token exists
	var userId int
	err1 := db.QueryRow(`SELECT user_id FROM auth_tokens WHERE token = ?`, authToken).Scan(&userId)
	if err1 != nil {
		return false, err1
	}

	//check is user is admin
	var userRole string
	err2 := db.QueryRow(`SELECT role FROM users WHERE id = ?`, userId).Scan(&userRole)
	if err2 != nil {
		return false, err2
	}

	return userRole == "admin", nil
}

func authenticateUser(db *sql.DB, authToken string) (int, error) {
	var userId int
	err := db.QueryRow(`SELECT user_id FROM auth_tokens WHERE token = ?`, authToken).Scan(&userId)
	return userId, err
}