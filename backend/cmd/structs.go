package main

import "time"

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenWrapper struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type FileWrapper struct {
	UUID        string    `json:"uuid"`
	DisplayName string    `json:"display_name"`
	Mime        *string   `json:"mime,omitempty"`
	SizeBytes   *int64    `json:"size_bytes,omitempty"`
	Sha256      *string   `json:"sha256,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type FolderWrapper struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type FolderContents struct {
	Folders []FolderWrapper `json:"folders"`
	Files   []FileWrapper   `json:"files"`
}

type UserInfoWrapper struct {
	Username   string `json:"username"`
	QuotaBytes string `json:"quota_bytes"`
	UsedBytes  string `json:"used_bytes"`
}

type UploadReq struct {
	Path       string `json:"path"`
	Filename   string `json:"filename"`
	Mime       string `json:"mime"`
	Size_bytes int64  `json:"size_bytes"`
	Sha256     string `json:"sha256"`
}
