---
layout: "mysql"
page_title: "MySQL: mysql_tables"
sidebar_current: "docs-mysql-datasource-tables"
description: |-
  Lists tables in a database on a MySQL server.
---

# mysql\_tables

The `mysql_tables` data source lists tables within a specified database on the MySQL server. Optionally filters by a `LIKE` pattern.

## Example Usage

### List all tables in a database

```hcl
data "mysql_tables" "app_tables" {
  database = "my_app"
}

output "tables" {
  value = data.mysql_tables.app_tables.tables
}
```

### Filter tables by pattern

```hcl
data "mysql_tables" "user_tables" {
  database = "my_app"
  pattern  = "user_%"
}
```

## Argument Reference

The following arguments are supported:

* `database` - (Required) The name of the database to list tables from.
* `pattern` - (Optional) A SQL `LIKE` pattern to filter table names (e.g., `user_%`, `%_archive`).

## Attributes Reference

* `tables` - A list of table names matching the query.
* `id` - A unique identifier for this data source instance.
