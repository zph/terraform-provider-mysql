---
layout: "mysql"
page_title: "MySQL: mysql_tables"
sidebar_current: "docs-mysql-datasource-tables"
description: |-
  Gets tables on a MySQL server.
---

# Data Source: mysql\_tables

The ``mysql_tables`` gets tables on a MySQL
server.

## Example Usage

```hcl
data "mysql_tables" "app" {
  database = "my_awesome_app"
}
```

## Argument Reference

The following arguments are supported:

* `database` - (Required) The name of the database.
* `pattern` - (Optional) Patterns for searching tables.

## Attributes Reference

The following attributes are exported:

* `tables` - The list of the table names.
