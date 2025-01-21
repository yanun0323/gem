package main

import (
	"log"

	gem "github.com/yanun0323/gem"
	"github.com/yanun0323/gem/example/model"
)

func main() {
	{
		sql := gem.New(&gem.MigratorConfig{
			Format:            gem.Goose,
			ExportDir:         "./example/export/goose",
			KeepDroppedColumn: true,
		})

		sql.Model(model.Model{})

		if err := sql.Run(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}

	{
		sql := gem.New(&gem.MigratorConfig{
			Format:            gem.GoMigrate,
			ExportDir:         "./example/export/go_migrate",
			KeepDroppedColumn: true,
		})

		sql.Model(model.Model{})

		if err := sql.Run(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}
}
