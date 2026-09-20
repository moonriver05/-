// Admin access is granted only by an operator with database access, never by public registration.
package main

import (
	"flag"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"mathwiki/internal/config"
	"mathwiki/internal/model"
	"strings"
)

func main() {
	username := flag.String("username", "", "Existing registered username")
	role := flag.String("role", "admin", "admin or learner")
	flag.Parse()
	if *username == "" || (*role != "admin" && *role != "learner") {
		log.Fatal("Usage: admin -username <registered-account> -role admin|learner")
	}
	cfg := config.Load()
	var d gorm.Dialector = mysql.Open(cfg.MySQLDSN)
	if cfg.DatabaseDriver == "sqlite" {
		d = sqlite.Open(cfg.SQLitePath)
	}
	db, err := gorm.Open(d, &gorm.Config{})
	if err != nil {
		log.Fatal("Cannot connect to database")
	}
	var u model.User
	if err = db.Where("username = ?", strings.ToLower(strings.TrimSpace(*username))).First(&u).Error; err != nil {
		log.Fatal("Account not found; register first")
	}
	if err = db.Model(&u).Update("role", *role).Error; err != nil {
		log.Fatal("Role update failed")
	}
	log.Printf("Account %s now has role %s. Refresh the page to apply.", *username, *role)
}
