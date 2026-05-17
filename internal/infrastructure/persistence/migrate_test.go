package persistence_test

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	persistence "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence"
)

func TestAutoMigrate_CreatesUsersTable(t *testing.T) {
	t.Parallel()

	t.Run("creates the users table with a unique index on email", func(t *testing.T) {
		t.Parallel()
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			t.Fatalf("open: %v", err)
		}

		if err := persistence.AutoMigrate(db); err != nil {
			t.Fatalf("migrate: %v", err)
		}

		var tableName string
		if err := db.Raw(
			"SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'users'",
		).Scan(&tableName).Error; err != nil {
			t.Fatalf("query sqlite_master: %v", err)
		}
		if tableName != "users" {
			t.Fatalf("users table not created: got %q", tableName)
		}

		type indexRow struct {
			Seq     int    `gorm:"column:seq"`
			Name    string `gorm:"column:name"`
			Unique  int    `gorm:"column:unique"`
			Origin  string `gorm:"column:origin"`
			Partial int    `gorm:"column:partial"`
		}
		var indexes []indexRow
		if err := db.Raw("PRAGMA index_list('users')").Scan(&indexes).Error; err != nil {
			t.Fatalf("pragma index_list: %v", err)
		}

		foundUniqueOnEmail := false
		for _, idx := range indexes {
			if idx.Unique != 1 {
				continue
			}
			type indexInfoRow struct {
				Seqno int    `gorm:"column:seqno"`
				Cid   int    `gorm:"column:cid"`
				Name  string `gorm:"column:name"`
			}
			var info []indexInfoRow
			if err := db.Raw("PRAGMA index_info('" + idx.Name + "')").Scan(&info).Error; err != nil {
				t.Fatalf("pragma index_info %q: %v", idx.Name, err)
			}
			if len(info) == 1 && info[0].Name == "email" {
				foundUniqueOnEmail = true
				break
			}
		}
		if !foundUniqueOnEmail {
			t.Errorf("no unique index covering email on users table; indexes=%+v", indexes)
		}
	})
}
