package main

import (
	"fmt"
	"os"

	_ "ariga.io/atlas-go-sdk/recordriver"
	"ariga.io/atlas-provider-gorm/gormschema"
	"ariga.io/atlas-provider-gorm/internal/testdata/models"

	// Sqlite driver based on CGO
	ckmodels "ariga.io/atlas-provider-gorm/internal/testdata/circularfks"
)

func main() {
	_, err := gormschema.New("mysql").
		Load(models.Pet{}, models.User{}, ckmodels.Event{}, ckmodels.Location{})

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load gorm schema: %v\n", err)
		os.Exit(1)
	}
	// io.WriteString(os.Stdout, stmts)

	err = gormschema.CreateTriggers(models.User{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create triggers: %v\n", err)
		os.Exit(1)
	}

}
