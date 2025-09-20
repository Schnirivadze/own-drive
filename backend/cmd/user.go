package main

import (
	"database/sql"
	"log"
	"time"
)

func getUserIdByLogin(db *sql.DB, username, password string) (int, error) {
	var userId int
	err := db.QueryRow("SELECT id FROM users WHERE username=? AND password=?", username, password).Scan(&userId)
	return userId, err
}

func createUser(db *sql.DB, username, password string, quotaBytes int64) error {
	result, err := db.Exec(`INSERT INTO users (username, password_hash, quota_bytes) VALUES (?, ?, ?)`, username, password, quotaBytes)
	log.Printf("CreateUser result: %v\n", result)
	return err
}

func createToken(db *sql.DB, userId int) (string, string, error) {
	var token string = ""
	var err error
	for {
		// generate token
		token, err = generateRawToken()
		if err != nil {
			return "", "", err
		}

		// check if token already exists
		var tempUserId int
		err1 := db.QueryRow("SELECT user_id FROM tokens WHERE token=?", token).Scan(&tempUserId)
		if err1 != sql.ErrNoRows {
			log.Printf("The token %s already exists\n", token)
			continue
		}

		// check if user already has a token
		var tempToken string
		var tempExpiryDate time.Time
		err2 := db.QueryRow("SELECT token, expires_at FROM tokens WHERE user_id = ?", userId).Scan(&tempToken, &tempExpiryDate)
		if  err2 == nil {
			// compare expiryDate with current time
			if time.Now().After(tempExpiryDate) {
				_, delErr := db.Exec("DELETE FROM tokens WHERE user_id = ?", userId)
				if delErr != nil {
					// create new token after deletion
					break
				}
			}

			// return token that user has and which isnt exired
			log.Printf("The user#%d already has a token: %s\n", userId, tempToken)
			return tempToken, tempExpiryDate.String(), nil

		}

		// break the loop if token is unique and user doesnt have any
		break
	}

	// set expiry date
	expiresAt := time.Now().Add(6 * time.Hour).UTC()

	// inster token in db
	_, err = db.Exec(`INSERT INTO tokens (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userId, expiresAt)
	if err != nil {
		return "", "", err
	}

	return token, expiresAt.String(), nil
}
