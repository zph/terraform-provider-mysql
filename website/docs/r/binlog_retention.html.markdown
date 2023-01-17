---
layout: "mysql"
page_title: "MySQL: mysql_binlog_retention"
sidebar_current: "docs-mysql-resource-binlog_retention"
description: |-
  Manages RDS mysql binlog retention period.
---

# mysql\_binlog\_retention

The ``mysql_binlog_retention`` resource manages binlog retention period (in hours) on a RDS MySQL
server.

~> **Note:** This resource only works with AMAZON RDS MySQL.

## Example Usage

```hcl
resource "mysql_binlog_retention" "this" {
  retention_period = 48
}
```

## Argument Reference

The following arguments are supported:

* `retention_period` - (Required) binlog retention period in hours

## Attributes Reference

No further attributes are exported.
