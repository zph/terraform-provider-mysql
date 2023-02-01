---
layout: "mysql"
page_title: "MySQL: mysql_rds_config"
sidebar_current: "docs-mysql-resource-mysql_rds_config"
description: |-
  Manages RDS mysql config.
---

# mysql\_rds\_config

The ``mysql_rds_config`` resource manages two configurations supported by AWS RDS MySQL
server.

~> **Note:** This resource only works with AMAZON RDS MySQL.

## Example Usage

```hcl
resource "mysql_rds_config" "this" {
  binlog_retention_period  = 48
  replication_target_delay = 30
}
```

## Argument Reference

The following arguments are supported:

* `binlog_retention_period` - (Optional) binlog retention period in hours
* `replication_target_delay` - (Optional) replicaation target delay in seconds

## Attributes Reference

No further attributes are exported.

## Import

RDS config can be imported with any ID name

Example Usage:

```terraform import mysql_rds_config.<tf_name> <any random ID>```
