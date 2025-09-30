package main

import (
	"database/sql"
	"log"
	"net/http"
)

const (
    DBPath     				= "./data/drive.db"
    StorageRoot   				= "./files"
    DefaultQuotaBytes   	= 25 * 1000 * 1000 * 1000
	InviteTokenValidHours 	= 24
	AuthTokenValidHours 	= 6
)
var DB *sql.DB;

func main() {
	log.Println("Starting server")

	log.Println("Opening db")
	var err error
	DB, err = openDB(DBPath)
	if err != nil { log.Fatal(err) }
	if DB == nil {log.Fatal("DB is nil\n")}

	log.Println("Running sql")
	if err := runSqlFromFile(DB,"./migrations/init.sql"); err != nil { log.Fatal(err) }
	if err := runSqlFromFile(DB,"./migrations/dummy.sql"); err != nil { log.Fatal(err) } // dummy data

	log.Println("Setting up handlers")
	// Users
	http.Handle("/api/users/login", corsMiddleware(http.HandlerFunc(handleAuth))) 				// POST
	http.Handle("/api/users/register", corsMiddleware(http.HandlerFunc(handleRegister))) 		// POST
	http.Handle("/api/users/me", corsMiddleware(http.HandlerFunc(handleUser))) 					// GET PATCH DELETE
	// Admin
	http.Handle("/api/admin/invites", corsMiddleware(http.HandlerFunc(handleInvites)))			// GET POST
	// Storage
	http.Handle("/api/storage/upload", corsMiddleware(http.HandlerFunc(handleUploads)))			// POST
	http.Handle("/api/storage/uploads/", corsMiddleware(http.HandlerFunc(handleUploadProcess)))	// PUT POST
	http.Handle("/api/storage/file/", corsMiddleware(http.HandlerFunc(handleFile)))				// GET PATCH DELETE
	http.Handle("/api/storage/files/", corsMiddleware(http.HandlerFunc(handleFiles)))			// GET POST PATCH DELETE

	log.Println("Server is up")
	log.Fatal((http.ListenAndServe(":8000", nil)))
}
