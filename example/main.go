package main

import (
	"log"

	"github.com/yanun0323/gem"
	"github.com/yanun0323/gem/example/model"
)

func main() {
	{
		sql := gem.New(&gem.Config{
			Tool:              gem.Goose,
			ExportDir:         "./example/export/goose",
			KeepDroppedColumn: true,
		})

		sql.AddModels(
			model.Model{},
			model.User{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}

	{
		sql := gem.New(&gem.Config{
			Tool:              gem.GolangMigrate,
			ExportDir:         "./example/export/go_migrate",
			KeepDroppedColumn: true,
		})

		sql.AddModels(
			model.Model{},
			model.User{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}
}
