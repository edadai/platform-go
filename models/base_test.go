package models

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestBaseModelBeforeCreateSetsUUID(t *testing.T) {
	var model BaseModel

	if err := model.BeforeCreate(&gorm.DB{}); err != nil {
		t.Fatal(err)
	}
	if model.ID == uuid.Nil {
		t.Fatal("ID was not set")
	}
}

func TestBaseModelWithUserBeforeCreateSetsCreatedBy(t *testing.T) {
	userID := uuid.Must(uuid.NewV7())
	db := (&gorm.DB{Statement: &gorm.Statement{Settings: sync.Map{}}}).Set("user_id", userID)

	var model BaseModelWithUser
	if err := model.BeforeCreate(db); err != nil {
		t.Fatal(err)
	}
	if model.CreatedBy == nil || *model.CreatedBy != userID {
		t.Fatalf("CreatedBy = %v, want %s", model.CreatedBy, userID)
	}
}
