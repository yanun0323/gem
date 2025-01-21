package main

import (
	"context"
	"log"

	gem "github.com/yanun0323/gem"
	"github.com/yanun0323/gem/example/model"
)

func main() {
	{
		sql := gem.New(&gem.MigratorConfig{
			Format:    gem.Goose,
			ExportDir: "./example/export/goose",
		})

		sql.Model(model.Model{})

		if err := sql.Run(context.Background()); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}

	{
		sql := gem.New(&gem.MigratorConfig{
			Format:    gem.GoMigrate,
			ExportDir: "./example/export/go_migrate",
		})

		sql.Model(model.Model{})

		if err := sql.Run(context.Background()); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}
}
