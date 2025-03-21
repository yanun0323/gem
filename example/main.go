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
			KeepDroppedColumn: false,
		})

		sql.AddModels(
			model.Model{},
			model.User{},
			model.UserAlias{},
			model.Address{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}

	{
		sql := gem.New(&gem.Config{
			Tool:              gem.GolangMigrate,
			OutputPath:        "./example/export/go_migrate",
			KeepDroppedColumn: false,
		})

		sql.AddModels(
			model.User{},
			model.Model{},
			model.UserAlias{},
			model.Address{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}

	{
		sql := gem.New(&gem.Config{
			Tool:              gem.RawSQL,
			OutputPath:        "./example/export/raw_sql",
			KeepDroppedColumn: false,
		})

		sql.AddModels(
			model.User{},
			model.Model{},
			model.UserAlias{},
			model.Address{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}

	{
		sql := gem.New(&gem.Config{
			Tool:              gem.RawSQL,
			OutputPath:        "./example/export/raw_sql_aggregation",
			KeepDroppedColumn: false,
			RawSQLAggregation: true,
		})

		sql.AddModels(
			model.User{},
			model.Model{},
			model.UserAlias{},
			model.Address{},
		)

		if err := sql.Generate(); err != nil {
			log.Fatalf("run migrator, err: %+v", err)
		}
	}
}
