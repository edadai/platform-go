package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BaseModelCreatedAt struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type BaseModel struct {
	BaseModelCreatedAt

	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type BaseModelWithUser struct {
	BaseModel

	CreatedBy *uuid.UUID `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy *uuid.UUID `gorm:"type:uuid" json:"updated_by,omitempty"`
	DeletedBy *uuid.UUID `gorm:"type:uuid" json:"deleted_by,omitempty"`
}

func (base *BaseModelCreatedAt) BeforeCreate(_ *gorm.DB) error {
	if base.ID == uuid.Nil {
		base.ID = uuid.Must(uuid.NewV7())
	}
	return nil
}

func (base *BaseModel) BeforeCreate(tx *gorm.DB) error {
	return base.BaseModelCreatedAt.BeforeCreate(tx)
}

func (base *BaseModelWithUser) BeforeCreate(tx *gorm.DB) error {
	if err := base.BaseModel.BeforeCreate(tx); err != nil {
		return err
	}

	if userID, ok := tx.Get("user_id"); ok {
		if id, valid := userID.(uuid.UUID); valid {
			base.CreatedBy = &id
		}
	}

	return nil
}
