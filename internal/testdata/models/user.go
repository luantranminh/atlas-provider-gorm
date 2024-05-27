package models

import (
	"time"

	"gorm.io/gorm"

	"ariga.io/atlas-provider-gorm/gormschema"
)

type User struct {
	gorm.Model
	Name string
	Age  int
	Pets []Pet
}

type WorkingAgedUsers struct {
	Name string
	Age  int
}

func (WorkingAgedUsers) ViewDef(dialect string) []gormschema.ViewOption {
	return []gormschema.ViewOption{
		gormschema.BuildStmt(func(db *gorm.DB) *gorm.DB {
			return db.Model(&User{}).Where("age BETWEEN 18 AND 65").Select("name, age")
		}),
	}
}

type UserPetHistory struct {
	UserID    uint
	PetID     uint
	CreatedAt time.Time
}

func (User) Triggers(dialect string) []gormschema.TriggerOption {
	return []gormschema.TriggerOption{
		gormschema.CreateStmt(`
			CREATE TRIGGER user_pet_history_trigger
			AFTER INSERT ON pets
			FOR EACH ROW
			BEGIN
				INSERT INTO user_pet_histories (user_id, pet_id, created_at) 
				VALUES (NEW.user_id, NEW.id, NOW(3));
			END
		`),
	}
}
