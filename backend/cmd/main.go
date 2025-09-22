package main

import (
	"database/sql"
	"log"
	"net/http"
)

const (
    DBPath     				= "./data/drive.db"
    FilesDir   				= "./files"
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
	http.HandleFunc("/api/auth",handleAuth)
	http.HandleFunc("/api/user",handleUser)
	http.HandleFunc("/api/admin",handleAdmin)


	log.Println("Server is up")
	log.Fatal((http.ListenAndServe(":8000", nil)))
}
