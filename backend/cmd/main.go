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
	http.Handle("/api/auth", corsMiddleware(http.HandlerFunc(handleAuth)))
	http.Handle("/api/user", corsMiddleware(http.HandlerFunc(handleUser)))
	http.Handle("/api/admin", corsMiddleware(http.HandlerFunc(handleAdmin)))
	http.Handle("/api/file/upload", corsMiddleware(http.HandlerFunc(handleFileUpload)))
	http.Handle("/api/file/download", corsMiddleware(http.HandlerFunc(handleFileDownload)))
	http.Handle("/api/file/list", corsMiddleware(http.HandlerFunc(handleFileList)))
	http.Handle("/api/file/delete", corsMiddleware(http.HandlerFunc(handleFileDelete)))

	log.Println("Server is up")
	log.Fatal((http.ListenAndServe(":8000", nil)))
}
