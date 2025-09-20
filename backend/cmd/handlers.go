package main

import (
	"encoding/json"
	"net/http"
)

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResp struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"` 
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	var login LoginReq
	if err := json.NewDecoder(r.Body).Decode(&login); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	userId, errLogin := getUserIdByLogin(DB, login.Username, login.Password)
	if errLogin != nil {
		http.Error(w, "Invalid login", http.StatusBadRequest)
		return
	}

	token, tokenExpiryDate,errToken := createToken(DB, userId)
	if errToken != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Authorization", token)
	w.Header().Set("Content-Type", "application/json")
	resp := LoginResp{
		Token:     token,
		ExpiresAt: tokenExpiryDate, 
	}
	json.NewEncoder(w).Encode(resp)
}
