package gormschema

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"embed"
	"errors"
	"fmt"
	"slices"
	"text/template"

	"ariga.io/atlas-go-sdk/recordriver"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	gormig "gorm.io/gorm/migrator"
)

// New returns a new Loader.
func New(dialect string, opts ...Option) *Loader {
	l := &Loader{dialect: dialect, config: &gorm.Config{}}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

type (
	// Loader is a Loader for gorm schema.
	Loader struct {
		dialect           string
		config            *gorm.Config
		beforeAutoMigrate []func(*gorm.DB) error
	}
	// Option configures the Loader.
	Option func(*Loader)
)

// WithConfig sets the gorm config.
func WithConfig(cfg *gorm.Config) Option {
	return func(l *Loader) {
		l.config = cfg
	}
}

// Load loads the models and returns the DDL statements representing the schema.
func (l *Loader) Load(models ...any) (string, error) {
	var di gorm.Dialector
	switch l.dialect {
	case "sqlite":
		rd, err := sql.Open("recordriver", "gorm")
		if err != nil {
			return "", err
		}
		di = sqlite.Dialector{Conn: rd}
		recordriver.SetResponse("gorm", "select sqlite_version()", &recordriver.Response{
			Cols: []string{"sqlite_version()"},
			Data: [][]driver.Value{{"3.30.1"}},
		})
	case "mysql":
		di = mysql.New(mysql.Config{
			DriverName: "recordriver",
			DSN:        "gorm",
		})
		recordriver.SetResponse("gorm", "SELECT VERSION()", &recordriver.Response{
			Cols: []string{"VERSION()"},
			Data: [][]driver.Value{{"8.0.24"}},
		})
	case "postgres":
		di = postgres.New(postgres.Config{
			DriverName: "recordriver",
			DSN:        "gorm",
		})
	case "sqlserver":
		di = sqlserver.New(sqlserver.Config{
			DriverName: "recordriver",
			DSN:        "gorm",
		})
	default:
		return "", fmt.Errorf("unsupported engine: %s", l.dialect)
	}
	db, err := gorm.Open(di, l.config)
	if err != nil {
		return "", err
	}
	if l.dialect != "sqlite" {
		db.Config.DisableForeignKeyConstraintWhenMigrating = true
	}
	for _, cb := range l.beforeAutoMigrate {
		if err = cb(db); err != nil {
			return "", err
		}
	}
	if err = db.AutoMigrate(models...); err != nil {
		return "", err
	}
	db, err = gorm.Open(dialector{
		Dialector: di,
	}, l.config)
	if err != nil {
		return "", err
	}
	cm, ok := db.Migrator().(*migrator)
	if !ok {
		return "", err
	}

	cm.CreateTriggers(models...)
	if !l.config.DisableForeignKeyConstraintWhenMigrating && l.dialect != "sqlite" {

		if err = cm.CreateConstraints(models); err != nil {
			return "", err
		}
	}
	s, ok := recordriver.Session("gorm")
	if !ok {
		return "", errors.New("gorm db session not found")
	}
	return s.Stmts(), nil
}

type migrator struct {
	gormig.Migrator
	dialectMigrator gorm.Migrator
}

type dialector struct {
	gorm.Dialector
}

// Migrator returns a new gorm.Migrator which can be used to automatically create all Constraints
// on existing tables.
func (d dialector) Migrator(db *gorm.DB) gorm.Migrator {
	return &migrator{
		Migrator: gormig.Migrator{
			Config: gormig.Config{
				DB:        db,
				Dialector: d,
			},
		},
		dialectMigrator: d.Dialector.Migrator(db),
	}
}

// HasTable always returns `true`. By returning `true`, gorm.Migrator will try to alter the table to add constraints.
func (m *migrator) HasTable(dst any) bool {
	return true
}

// CreateConstraints detects constraints on the given model and creates them using `m.dialectMigrator`.
func (m *migrator) CreateConstraints(models []any) error {
	for _, model := range m.ReorderModels(models, true) {
		err := m.Migrator.RunWithValue(model, func(stmt *gorm.Statement) error {

			relationNames := make([]string, 0, len(stmt.Schema.Relationships.Relations))
			for name := range stmt.Schema.Relationships.Relations {
				relationNames = append(relationNames, name)
			}
			// since Relations is a map, the order of the keys is not guaranteed
			// so we sort the keys to make the sql output deterministic
			slices.Sort(relationNames)

			for _, name := range relationNames {
				rel := stmt.Schema.Relationships.Relations[name]

				if rel.Field.IgnoreMigration {
					continue
				}
				if constraint := rel.ParseConstraint(); constraint != nil &&
					constraint.Schema == stmt.Schema {
					if err := m.dialectMigrator.CreateConstraint(model, constraint.Name); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *migrator) CreateTriggers(models ...any) error {
	for _, model := range models {
		model, hasTrigger := model.(interface{ Triggers() []Trigger })
		if !hasTrigger {
			continue
		}

		for _, trigger := range model.Triggers() {
			stmt, err := trigger.String(m.Dialector.Name())
			if err != nil {
				return err
			}
			if err = m.DB.Exec(stmt).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func CreateTriggers(models ...any) error {
	for _, model := range models {
		model, hasTrigger := model.(interface{ Triggers() []Trigger })
		if !hasTrigger {
			continue
		}
		for _, trigger := range model.Triggers() {
			stmt, err := trigger.String("mysql")
			if err != nil {
				return err
			}
			fmt.Println(stmt)
		}
	}

	return nil
}

// WithJoinTable sets up a join table for the given model and field.
func WithJoinTable(model any, field string, jointable any) Option {
	return func(l *Loader) {
		l.beforeAutoMigrate = append(l.beforeAutoMigrate, func(db *gorm.DB) error {
			return db.SetupJoinTable(model, field, jointable)
		})
	}
}

type TriggerTime string
type TriggerFor string
type TriggerEvent string

const (
	TriggerBefore    TriggerTime = "BEFORE"
	TriggerAfter     TriggerTime = "AFTER"
	TriggerInsteadOf TriggerTime = "INSTEAD OF"
)

const (
	TriggerForRow  TriggerFor = "ROW"
	TriggerForStmt TriggerFor = "STATEMENT"
)

const (
	TriggerEventInsert   TriggerEvent = "INSERT"
	TriggerEventUpdate   TriggerEvent = "UPDATE"
	TriggerEventDelete   TriggerEvent = "DELETE"
	TriggerEventTruncate TriggerEvent = "TRUNCATE"
)

type Trigger struct {
	Name       string
	ActionTime TriggerTime  // BEFORE, AFTER, or INSTEAD OF.
	Event      TriggerEvent // INSERT, UPDATE, DELETE, etc.
	For        TriggerFor   // FOR EACH ROW or FOR EACH STATEMENT.
	Body       string       // Trigger body only.
}

//go:embed templates/triggers/*.tmpl
var triggerTemplates embed.FS

func (t Trigger) String(dialect string) (string, error) {
	tmpl, err := template.ParseFS(triggerTemplates, fmt.Sprintf("templates/triggers/%s.tmpl", dialect))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t); err != nil {
		return "", err
	}
	return buf.String(), nil
}
