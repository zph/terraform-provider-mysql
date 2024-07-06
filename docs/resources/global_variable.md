---
layout: "mysql"
page_title: "MySQL: mysql_global_variable"
sidebar_current: "docs-mysql-resource-global-variable"
description: |-
  Manages a global variables on a MySQL server.
---

# mysql\_global\_variable

The ``mysql_global_variable`` resource manages a global variables on a MySQL
server.

~> **Note on MySQL:** MySQL global variables are [not persistent](https://dev.mysql.com/doc/refman/5.7/en/set-variable.html)

~> **Note on TiDB:** TiDB global variables are [persistent](https://docs.pingcap.com/tidb/v5.4/sql-statement-set-variable#mysql-compatibility)

~> **Note about `destroy`:** `destroy` will try assign `DEFAULT` value for global variable.
  Unfortunately not every variable support this.

## Example Usage

```hcl
resource "mysql_global_variable" "max_connections" {
  name = "max_connections"
  value = "100"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required) The name of the global variable.
* `value` - (Required) The value of the global variable.

## Attributes Reference

No further attributes are exported.

## Import

Global variable can be imported using global variable name.

```shell
$ terraform import mysql_global_variable.max_connections max_connections
```
