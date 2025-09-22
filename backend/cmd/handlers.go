package main

import (
	"encoding/json"
	"log"
	"net/http"
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

func handleAuth(w http.ResponseWriter, r *http.Request) {
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
		err := updateUser(DB, r.Header.Get("Authorization"),newLogin.Username, newLogin.Password)
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
