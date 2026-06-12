---
layout: "mysql"
page_title: "MySQL: mysql_databases"
sidebar_current: "docs-mysql-datasource-databases"
description: |-
  Lists databases on a MySQL server.
---

# mysql\_databases

The `mysql_databases` data source lists databases available on the MySQL server. Optionally filters by a `LIKE` pattern.

## Example Usage

### List all databases

```hcl
data "mysql_databases" "all" {}

output "all_databases" {
  value = data.mysql_databases.all.databases
}
```

### Filter databases by pattern

```hcl
data "mysql_databases" "app_dbs" {
  pattern = "app_%"
}
```

## Argument Reference

The following arguments are supported:

* `pattern` - (Optional) A SQL `LIKE` pattern to filter database names (e.g., `app_%`, `%_production`).

## Attributes Reference

* `databases` - A list of database names matching the query.
* `id` - A unique identifier for this data source instance.
