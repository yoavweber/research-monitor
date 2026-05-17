package persistence_test

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

func TestAutoMigrate_UsersTable(t *testing.T) {
	t.Parallel()

	t.Run("creates the users table with the model registered", func(t *testing.T) {
		t.Parallel()
		db := testdb.New(t)

		if !db.Migrator().HasTable(&userpersist.Model{}) {
			t.Fatal("users table not created by AutoMigrate")
		}
	})

	t.Run("rejects a duplicate email at the persistence layer", func(t *testing.T) {
		t.Parallel()
		db := testdb.New(t)
		now := time.Now().UTC()
		first := userpersist.Model{
			ID:           "11111111-1111-1111-1111-111111111111",
			Email:        "duplicate@example.com",
			PasswordHash: "hash-1",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		second := userpersist.Model{
			ID:           "22222222-2222-2222-2222-222222222222",
			Email:        "duplicate@example.com",
			PasswordHash: "hash-2",
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		if err := db.Create(&first).Error; err != nil {
			t.Fatalf("insert first: %v", err)
		}
		err := db.Create(&second).Error

		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			t.Fatalf("insert duplicate email: got %v, want gorm.ErrDuplicatedKey", err)
		}
	})
}
