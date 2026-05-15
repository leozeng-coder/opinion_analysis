package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"opinion-analysis/config"
)

func dbNameFromDSN(dsn string) string {
	i := strings.Index(dsn, ")/")
	if i < 0 {
		return "opinion_analysis"
	}
	i += 2
	rest := dsn[i:]
	j := strings.Index(rest, "?")
	if j < 0 {
		return rest
	}
	return rest[:j]
}

func main() {
	config.Load("config/config.yaml")
	dsn := config.Cfg.Database.DSN
	dbName := dbNameFromDSN(dsn)
	if dbName == "" {
		log.Fatal("could not parse database name from DSN")
	}
	adminDSN := strings.Replace(dsn, "/"+dbName, "/mysql", 1)

	db, err := sql.Open("mysql", adminDSN)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}
	_, err = db.Exec(fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		dbName,
	))
	if err != nil {
		log.Fatalf("create database: %v", err)
	}
	fmt.Printf("database %q is ready\n", dbName)
}
