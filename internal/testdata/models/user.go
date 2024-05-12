package models

import (
	"ariga.io/atlas-provider-gorm/gormschema"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Name string
	Pets []Pet
}

func (User) Triggers() []gormschema.Trigger {
	return []gormschema.Trigger{
		{
			Name:       "user_audit",
			ActionTime: gormschema.TriggerAfter,
			Event:      gormschema.TriggerEventInsert,
			For:        gormschema.TriggerForRow,
			Body:       `INSERT INTO user_audit (name) VALUES (NEW.name)`,
		},
	}
}
