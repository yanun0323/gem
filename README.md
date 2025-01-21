# Gem

Gem is a powerful database migration file generator for Go applications using [GORM](https://gorm.io) tags. It simplifies the process of creating database migration files by automatically generating SQL statements based on your Go struct definitions.

## Features

- Supports multiple migration formats:
  - Raw SQL
  - [Goose](https://github.com/pressly/goose)
  - [Golang-Migrate](https://github.com/golang-migrate/migrate)
- Automatically generates:
  - Table creation statements
  - Column definitions with constraints
  - Indexes (normal and unique)
  - Foreign keys
- Tracks schema changes and generates migration files only when needed
- Preserves migration history
- Supports complex data types and relationships
- Handles embedded structs and custom table names

## Installation

```bash
go get github.com/yanun0323/gem@latest
```

## Usage

### Basic Example

```go
package main

import (
    "log"
    "github.com/yanun0323/gem"
)

type User struct {
    ID        uint      `gorm:"primaryKey;autoIncrement"`
    Name      string    `gorm:"size:255;not null"`
    Email     string    `gorm:"unique;size:100"`
    CreatedAt time.Time `gorm:"autoCreateTime"`
}

func main() {
    g := gem.New(&gem.Config{
        Format:    gem.Goose,        // or gem.GolangMigrate, gem.RawSQL
        ExportDir: "./migrations",
        KeepDroppedColumn: false,
    })

    g.AddModels(User{})

    if err := g.Generate(); err != nil {
        log.Fatal(err)
    }
}
```

### Configuration Options

```go
type Config struct {
    Format            MigrationTool // Goose, GoMigrate, or RawSQL
    ExportDir         string        // Directory to store migration files
    KeepDroppedColumn bool          // Keep dropped columns in down migrations
}
```

### Supported GORM Tags

|   Tag Name    | Description                     |
| :-----------: | :------------------------------ |
|    column     | Column database name            |
|     type      | Column data type                |
|     size      | Column size/length              |
|  primaryKey   | Specifies column as primary key |
|    unique     | Specifies column as unique      |
|     index     | Creates index                   |
|  uniqueIndex  | Creates unique index            |
|    default    | Specifies default value         |
|   not null    | Specifies NOT NULL constraint   |
| autoIncrement | Enables auto-increment          |
|   embedded    | Embeds the field                |
|    comment    | Adds column comment             |

For a complete list of supported tags, please refer to [tag.md](tag.md).

## Example Project Structure

```
.
├── migrations/
│   ├── 20240101000000_create_users.sql
│   └── .gem/
│       └── snapshots.json
├── models/
│   └── user.go
└── main.go
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
