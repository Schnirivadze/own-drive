package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
)

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
	if err := runSqlFromFile(DB, "./migrations/cleanTokens.sql"); err != nil {
		log.Printf("Couldnt clear tokens: %s", err.Error())
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
		http.Error(w, "Invalid login", http.StatusUnauthorized)
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
	resp := TokenWrapper{
		Token:     token,
		ExpiresAt: tokenExpiryDate,
	}
	json.NewEncoder(w).Encode(resp)
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	// Decode registration data
	var register LoginReq
	if err := json.NewDecoder(r.Body).Decode(&register); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Verify invite token and create user
	if err := createUser(DB, r.Header.Get("Authorization"), register.Username, register.Password); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Invalid invite", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func handleUser(w http.ResponseWriter, r *http.Request) {
	// Authenticate user
	userId, errAuth := authenticateUser(DB, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {

	case http.MethodGet:
		// Get user info from db
		var userInfo UserInfoWrapper
		if err := DB.QueryRow(`SELECT username, quota_bytes, used_bytes FROM users WHERE id=?`, userId).Scan(&userInfo.Username, &userInfo.QuotaBytes, &userInfo.UsedBytes); err != nil {
			http.Error(w, "Get user info failed", http.StatusInternalServerError)
			return
		}

		// Convert to JSON and send
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userInfo)

	case http.MethodPatch:
		// Decode new data
		var newLogin LoginReq
		if err := json.NewDecoder(r.Body).Decode(&newLogin); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Update user
		err := updateUser(DB, userId, newLogin.Username, newLogin.Password)
		if err != nil {
			http.Error(w, "Update user failed", http.StatusInternalServerError)
			return
		}

	case http.MethodDelete:
		// Delete user
		if err := deleteUser(DB, r.Header.Get("Authorization")); err != nil {
			http.Error(w, "Delete user failed", http.StatusInternalServerError)
			return
		}

		// Delete users files
		if err := deleteFolder(DB, "~", userId); err != nil {
			http.Error(w, "Delete users files failed", http.StatusInternalServerError)
			return
		}

	default:
		http.Error(w, "Invalid method", http.StatusBadRequest)
	}
}

func handleInvites(w http.ResponseWriter, r *http.Request) {
	// Authenticate admin token
	isAdmin, errAuth := authenticateAdmin(DB, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Authenticate admin failed", http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		//Query db for tokens
		inviteTokens := make([]TokenWrapper, 0)
		rows, errQuery := DB.Query(`SELECT token, expires_at FROM invite_tokens`)
		if errQuery != nil {
			http.Error(w, "Query db for invite tokens failed", http.StatusInternalServerError)
			return
		}

		// Process tokens into structs
		for rows.Next() {
			var token TokenWrapper
			if err := rows.Scan(&token.Token, &token.ExpiresAt); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			inviteTokens = append(inviteTokens, token)
		}

		// Senf tokens as json array
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(inviteTokens)

	case http.MethodPost:
		// Create invite token
		inviteToken, errCreateInvite := createInviteToken(DB)
		if errCreateInvite != nil {
			http.Error(w, "Create invite failed", http.StatusInternalServerError)
			return
		}

		// Send invite token
		w.Header().Set("Content-Type", "application/json")
		resp := TokenWrapper{
			Token: inviteToken,
		}
		json.NewEncoder(w).Encode(resp)

	default:
		http.Error(w, "Invalid method", http.StatusBadRequest)
		return
	}

}

func handleUploads(w http.ResponseWriter, r *http.Request) {
	// Authenticate auth token
	userId, errAuth := authenticateUser(DB, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodPost:
		// Get upload data
		var upload UploadReq
		if err := json.NewDecoder(r.Body).Decode(&upload); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Register an upload
		uuid, errUploadStart := startUpload(DB, userId, upload)
		if errUploadStart != nil {
			http.Error(w, "Start upload failed", http.StatusInternalServerError)
			return
		}

		// Respond with uuid
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"upload_id":"` + uuid + `"}`))
	default:
		http.Error(w, "Invalid method", http.StatusBadRequest)
		return
	}
}

func handleUploadProcess(w http.ResponseWriter, r *http.Request) {
	// Authenticate auth token
	userId, errAuth := authenticateUser(DB, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get uuid (/api/storage/uploads/{uuid})
	lastSlashIndex := strings.LastIndex(r.URL.Path, "/")
	uuid := r.URL.Path[lastSlashIndex+1:]

	// Authenticate uuid
	var uuidValid bool
	DB.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=? AND owner_id=?)", uuid, userId).Scan(&uuidValid)
	if !uuidValid {
		http.Error(w, "Invalid UUID", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodPut:
		// Upload chunk
		if err := uploadChunk(DB, uuid, r.URL.Query().Get("offset"), r.Body); err != nil {
			http.Error(w, "Upload chunk failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	case http.MethodPost:
		// Finish upload
		if err := finishUpload(DB, uuid, userId); err != nil {
			http.Error(w, "Finish upload failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Invalid method", http.StatusBadRequest)
		return
	}
}

func handleFile(w http.ResponseWriter, r *http.Request) {
	// Get uuid (/api/storage/file/{uuid})
	lastSlashIndex := strings.LastIndex(r.URL.Path, "/")
	uuid := r.URL.Path[lastSlashIndex+1:]

	switch r.Method {
	case http.MethodGet:
		// Authenticate user
		userId, err := authenticateUser(DB, r.URL.Query().Get("auth"))
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Authenticate uuid
		var uuidValid bool
		DB.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=? AND owner_id=?)", uuid, userId).Scan(&uuidValid)
		if !uuidValid {
			http.Error(w, "Invalid UUID", http.StatusBadRequest)
			return
		}

		file, mime, safeName, modTime, errGetFile := getFileByUUID(DB, userId, uuid)
		if errGetFile != nil {
			http.Error(w, "Get file failed", http.StatusInternalServerError)
			return

		}
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(safeName))
		// Set headers
		if mime != "" {
			w.Header().Set("Content-Type", mime)
		} else {
			w.Header().Set("Content-Type", "application/octet-stream")
		}

		// http.ServeContent uses the provided modtime; we can use fi.ModTime()
		http.ServeContent(w, r, safeName, modTime, file)

	case http.MethodPatch:
		// Authenticate user
		userId, err := authenticateUser(DB, r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Authenticate uuid
		var uuidValid bool
		DB.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=? AND owner_id=?)", uuid, userId).Scan(&uuidValid)
		if !uuidValid {
			http.Error(w, "Invalid UUID", http.StatusInternalServerError)
			return
		}

		// Rename file
		if err := renameFile(DB, uuid, r.URL.Query().Get("name")); err != nil {
			http.Error(w, "Rename file failed", http.StatusInternalServerError)
			return
		}

	case http.MethodDelete:
		// Authenticate user
		userId, err := authenticateUser(DB, r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Authenticate uuid
		var uuidValid bool
		DB.QueryRow("SELECT EXISTS(SELECT 1 FROM files WHERE uuid=? AND owner_id=?)", uuid, userId).Scan(&uuidValid)
		if !uuidValid {
			http.Error(w, "Invalid UUID", http.StatusInternalServerError)
			return
		}

		// Delete file
		if err := deleteFile(DB, userId, uuid); err != nil {
			http.Error(w, "Delete file failed", http.StatusInternalServerError)
			return
		}

	default:
		http.Error(w, "Invalid method", http.StatusBadRequest)
		return
	}
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	// Get path (/api/storage/files/{path})
	var endpoint = "/api/storage/files/"
	pathStartIndex := len(endpoint)
	pathToFolder := r.URL.Path[pathStartIndex:]

	// Authenticate auth token
	userId, errAuth := authenticateUser(DB, r.Header.Get("Authorization"))
	if errAuth != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get folder contents
		contents, errList := listFolderContents(DB, pathToFolder, userId)
		if errList != nil {
			http.Error(w, "Geting folder contents failed", http.StatusInternalServerError)
			return
		}
		// Marshal to JSON
		out, err := json.Marshal(contents)
		if err != nil {
			http.Error(w, "Marshal contents to json failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(out)

	case http.MethodPost:
		if err := createFolder(DB, pathToFolder, userId); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	case http.MethodDelete:
		if err := deleteFolder(DB, pathToFolder, userId); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	case http.MethodPatch:
		if err := renameFolder(DB, pathToFolder, r.URL.Query().Get("name"), userId); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Unsupported method", http.StatusBadRequest)
		return
	}
}
