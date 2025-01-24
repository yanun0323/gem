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
			OutputPath:        "./example/export/goose",
			KeepDroppedColumn: true,
		})

		sql.AddModels(
			model.Model{},
			model.User{},
			model.UserAlias{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}

	{
		sql := gem.New(&gem.Config{
			Tool:              gem.GolangMigrate,
			OutputPath:        "./example/export/go_migrate",
			KeepDroppedColumn: true,
		})

		sql.AddModels(
			model.User{},
			model.Model{},
			model.UserAlias{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}
}
