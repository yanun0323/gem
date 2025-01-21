# Supported Tags

| Tag Name | Description |
|:-:|:-|
| column | column db name |
| type | column data type, prefer to use compatible general type, e.g: bool, int, uint, float, string, time, bytes, which works for all databases, and can be used with other tags together, like not null, size, autoIncrement… specified database data type like varbinary(8) also supported, when using specified database data type, it needs to be a full database data type, for example: MEDIUMINT UNSIGNED NOT NULL AUTO_INCREMENT |
| serializer | specifies serializer for how to serialize and deserialize data into db, e.g: serializer:json/gob/unixtime |
| size | specifies column data size/length, e.g: size:256 |
| primaryKey | specifies column as primary key |
| unique | specifies column as unique |
| default | specifies column default value |
| precision | specifies column precision |
| scale | specifies column scale |
| not null | specifies column as NOT NULL |
| autoIncrement | specifies column auto incrementable |
| autoIncrementIncrement | auto increment step, controls the interval between successive column values |
| embedded | embed the field |
| embeddedPrefix | column name prefix for embedded fields |
| autoCreateTime | track current time when creating, for int fields, it will track unix seconds, use value nano/milli to track unix nano/milli seconds, e.g: autoCreateTime:nano |
| autoUpdateTime | track current time when creating/updating, for int fields, it will track unix seconds, use value nano/milli to track unix nano/milli seconds, e.g: autoUpdateTime:milli |
| index | create index with options, use same name for multiple fields creates composite indexes, refer Indexes for details |
| uniqueIndex | same as index, but create uniqued index |
| check | creates check constraint, eg: check:age > 13, refer Constraints |
| <- | set field's write permission, <-:create create-only field, <-:update update-only field, <-:false no write permission, <- create and update permission |
| -> | set field's read permission, ->:false no read permission |
| - | ignore this field, - no read/write permission, -:migration no migrate permission, -:all no read/write/migrate permission |
| comment | add comment for field when migration |