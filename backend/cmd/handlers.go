package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthToken struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}
type InviteToken struct {
	Token string `json:"token"`
}

type FileJSONWrapper struct {
	UUID        string    `json:"uuid"`
	FolderID    *int64    `json:"folder_id,omitempty"`
	DisplayName string    `json:"display_name"`
	Mime        *string   `json:"mime,omitempty"`
	SizeBytes   *int64    `json:"size_bytes,omitempty"`
	Sha256      *string   `json:"sha256,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		// Preflight request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	// Clean db from expired or invalid tokens
	err0 := runSqlFromFile(DB, "./migrations/cleanTokens.sql")
	if err0 != nil {
		log.Printf("Couldnt clear tokens: %s", err0.Error())
	}

	// Get login data
	var login LoginReq
	if err := json.NewDecoder(r.Body).Decode(&login); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Authenticate login
	userId, errLogin := getUserIdByLogin(DB, login.Username, login.Password)
	if errLogin != nil {
		http.Error(w, "Invalid login", http.StatusBadRequest)
		return
	}

	// Create auth token
	token, tokenExpiryDate, errToken := createAuthToken(DB, userId)
	if errToken != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Send auth token
	w.Header().Add("Authorization", token)
	w.Header().Set("Content-Type", "application/json")
	resp := AuthToken{
		Token:     token,
		ExpiresAt: tokenExpiryDate,
	}
	json.NewEncoder(w).Encode(resp)
}

func handleUser(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// decode registration data
		var register LoginReq
		if err := json.NewDecoder(r.Body).Decode(&register); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// verify invite token and create user
		if err := createUser(DB, r.Header.Get("Authorization"), register.Username, register.Password); err != nil {
			log.Printf("Server error: %s", err.Error())
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	case http.MethodPut:
		// decode registration data
		var newLogin LoginReq
		if err := json.NewDecoder(r.Body).Decode(&newLogin); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// verify invite token and update user
		err := updateUser(DB, r.Header.Get("Authorization"), newLogin.Username, newLogin.Password)
		if err != nil {
			log.Printf("Couldnt update user: %s", err.Error())
		}
	case http.MethodDelete:
		err := deleteUser(DB, r.Header.Get("Authorization"))
		if err != nil {
			log.Printf("Couldnt delete user: %s", err.Error())
		}

	default:
		log.Printf("User handler unknown method: %s\n", r.Method)
		w.WriteHeader(http.StatusBadRequest)
	}
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {

	action := r.URL.Query().Get("action")
	switch action {
	case "create-invite-token":
		// Clean db from expired or invalid tokens
		err0 := runSqlFromFile(DB, "./migrations/cleanTokens.sql")
		if err0 != nil {
			log.Printf("Couldnt clear tokens: %s", err0.Error())
		}

		// get admin token
		adminToken := r.Header.Get("Authorization")

		// authenticate token
		isAdmin, err1 := authenticateAdmin(DB, adminToken)
		if err1 != nil {
			log.Printf("Couldnt authenticate admin: %s", err1.Error())
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		if !isAdmin {
			log.Println("Not an admin")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// create invite token
		inviteToken, err2 := createInviteToken(DB)
		if err2 != nil {
			log.Printf("Failed to create token: %s\n", err2.Error())
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		// send invite token
		w.Header().Set("Content-Type", "application/json")
		resp := InviteToken{
			Token: inviteToken,
		}

		json.NewEncoder(w).Encode(resp)

	default:
		log.Printf("Admin handler unknown action: %s\n", action)
		w.WriteHeader(http.StatusBadRequest)
	}
}

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		switch r.URL.Query().Get("action") {
		case "single":
			uploadFile(DB, w, r)
		case "start":
			startUpload(DB, w, r)
		case "finish":
			finishUpload(DB, w, r)
		default:
			log.Printf("File upload handler unknown action: %s\n", r.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	case http.MethodPut:
		uploadChunk(DB, w, r)
	default:
		log.Printf("File upload handler unknown method: %s\n", r.Method)
		w.WriteHeader(http.StatusBadRequest)
	}
}

func handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("uuid") {
		downloadFileByUUID(DB, w, r)
	} else {
		http.Error(w, "missing uuid", http.StatusBadRequest)
	}
}

func handleFileList(w http.ResponseWriter, r *http.Request) {
	// Authenticate auth token
	userId, errAuth := authenticateUser(DB, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized: "+errAuth.Error(), http.StatusUnauthorized)
		return
	}

	// Get folder id
	path := r.URL.Query().Get("path")
	folderId, errFolderId := getFolderIdFromPath(DB, path, userId)
	if errFolderId != nil {
		http.Error(w, errFolderId.Error(), http.StatusInternalServerError)
		return
	}

	// Get folder contents
	rows, err := DB.Query(`
			SELECT uuid, folder_id, display_name, mime, size_bytes, sha256, created_at
			FROM files
			WHERE owner_id = ? AND folder_id = ? AND deleted_at IS NULL
		`, userId, folderId)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Turn sql rows into structs
	files := make([]FileJSONWrapper, 0)
	for rows.Next() {
		var f FileJSONWrapper
		var (
			dbFolderID  sql.NullInt64
			dbMime      sql.NullString
			dbSizeBytes sql.NullInt64
			dbSha256    sql.NullString
		)

		if err := rows.Scan(
			&f.UUID,
			&dbFolderID,
			&f.DisplayName,
			&dbMime,
			&dbSizeBytes,
			&dbSha256,
			&f.CreatedAt,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Map nullable columns into pointer fields
		if dbFolderID.Valid {
			f.FolderID = &dbFolderID.Int64
		} else {
			f.FolderID = nil
		}
		if dbMime.Valid {
			f.Mime = &dbMime.String
		}
		if dbSizeBytes.Valid {
			f.SizeBytes = &dbSizeBytes.Int64
		}
		if dbSha256.Valid {
			f.Sha256 = &dbSha256.String
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Marshal to JSON
	out, err := json.Marshal(files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send json list
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(out)
}
