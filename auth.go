package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const kanbanAuthCookie = "kanban_auth_token"

type authVerifyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	} `json:"data"`
}

func authBaseURL() string {
	base := strings.TrimRight(os.Getenv("AuthURL"), "/")
	if base == "" {
		return "https://auth.justdrink.com.tw"
	}
	return base
}

func verifyAuthToken(token string) (string, string, error) {
	body, _ := json.Marshal(map[string]string{"token": token})
	resp, err := http.Post(authBaseURL()+"/valifytoken", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("auth server returned %s", resp.Status)
	}
	var result authVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.Success || result.Data.UserID == "" {
		return "", "", fmt.Errorf("invalid auth token")
	}
	return result.Data.UserID, result.Data.Email, nil
}

func writeAuthJSON(w http.ResponseWriter, status int, userID, email string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": status < 400, "user_id": userID, "email": email})
}

func (h *Handler) CreateAuthSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeAuthJSON(w, http.StatusBadRequest, "", "")
		return
	}
	userID, email, err := verifyAuthToken(strings.TrimSpace(req.Token))
	if err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, "", "")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: kanbanAuthCookie, Value: req.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil, Expires: time.Now().Add(24 * time.Hour)})
	writeAuthJSON(w, http.StatusOK, userID, email)
}

func (h *Handler) CurrentAuthSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(kanbanAuthCookie)
	if err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, "", "")
		return
	}
	userID, email, err := verifyAuthToken(cookie.Value)
	if err != nil {
		writeAuthJSON(w, http.StatusUnauthorized, "", "")
		return
	}
	writeAuthJSON(w, http.StatusOK, userID, email)
}

func (h *Handler) DeleteAuthSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: kanbanAuthCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil})
	writeAuthJSON(w, http.StatusOK, "", "")
}
