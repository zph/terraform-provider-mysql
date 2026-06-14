---
layout: "mysql"
page_title: "MySQL: mysql_ti_database_placement_policy"
sidebar_current: "docs-mysql-resource-ti-database-placement-policy"
description: |-
  Applies a TiDB placement policy to an existing database.
---

# mysql\_ti\_database\_placement\_policy

The `mysql_ti_database_placement_policy` resource applies a TiDB placement policy to an existing database using [`ALTER DATABASE ... PLACEMENT POLICY`](https://docs.pingcap.com/tidb/stable/placement-rules-in-sql/).

~> **Note:** This resource manages only the placement policy attachment. Destroying it resets the database placement policy to TiDB's `default` policy; it does not drop the database.

~> **Note:** Do not manage the same database placement policy with both this resource and `mysql_database.placement_policy`.

## TiDB Semantics

This resource is an attachment workaround for TiDB's placement DDL. TiDB does not provide a separate database-placement attachment object, so the provider manages the direct assignment with `ALTER DATABASE ... PLACEMENT POLICY`.

On destroy, the provider runs `ALTER DATABASE ... PLACEMENT POLICY=default`. This removes the explicit database placement policy and does not restore any previously configured policy.

Readback uses `information_schema.schemata.TIDB_PLACEMENT_POLICY_NAME` when available. A `NULL` value is represented as `default` in Terraform state, meaning there is no direct database policy assignment.

## Example Usage

```hcl
resource "mysql_ti_placement_policy" "regional" {
  name           = "regional"
  primary_region = "us-east-1"
  regions        = ["us-east-1", "us-west-2"]
}

resource "mysql_database" "app" {
  name = "my_app"
}

resource "mysql_ti_database_placement_policy" "app" {
  database         = mysql_database.app.name
  placement_policy = mysql_ti_placement_policy.regional.name
}
```

## Argument Reference

The following arguments are supported:

* `database` - (Required, ForceNew) Database to configure.
* `placement_policy` - (Required) Placement policy to apply. Use `default` to reset the explicit database placement policy.

## Attributes Reference

* `id` - The database name.

## Import

Database placement policies can be imported using the database name.

```shell
terraform import mysql_ti_database_placement_policy.app my_app
```
