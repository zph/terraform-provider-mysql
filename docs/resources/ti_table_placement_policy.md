---
layout: "mysql"
page_title: "MySQL: mysql_ti_table_placement_policy"
sidebar_current: "docs-mysql-resource-ti-table-placement-policy"
description: |-
  Applies a TiDB placement policy to an existing table.
---

# mysql\_ti\_table\_placement\_policy

The `mysql_ti_table_placement_policy` resource applies a TiDB placement policy to an existing table using [`ALTER TABLE ... PLACEMENT POLICY`](https://docs.pingcap.com/tidb/stable/placement-rules-in-sql/).

~> **Note:** This resource manages only the placement policy attachment. Destroying it resets the table placement policy to TiDB's `default` policy; it does not drop the table.

## TiDB Semantics

This resource is an attachment workaround for TiDB's placement DDL. TiDB does not provide a separate table-placement attachment object, so the provider manages the direct assignment with `ALTER TABLE ... PLACEMENT POLICY`.

On destroy, the provider runs `ALTER TABLE ... PLACEMENT POLICY=default`. This removes the explicit table placement policy and does not restore any previously configured policy. After reset, TiDB can apply placement inherited from the database, range, or global default.

Readback uses `information_schema.tables.TIDB_PLACEMENT_POLICY_NAME`. A `NULL` value is represented as `default` in Terraform state, meaning there is no direct table policy assignment. It does not necessarily mean the table has no effective placement policy.

## Example Usage

```hcl
resource "mysql_ti_placement_policy" "regional" {
  name           = "regional"
  primary_region = "us-east-1"
  regions        = ["us-east-1", "us-west-2"]
}

resource "mysql_ti_table_placement_policy" "orders" {
  database         = "my_app"
  table            = "orders"
  placement_policy = mysql_ti_placement_policy.regional.name
}
```

## Argument Reference

The following arguments are supported:

* `database` - (Required, ForceNew) Database containing the table.
* `table` - (Required, ForceNew) Table to configure.
* `placement_policy` - (Required) Placement policy to apply. Use `default` to reset the explicit table placement policy.

## Attributes Reference

* `id` - The table placement attachment ID, formatted as `<database>.<table>`.

## Import

Table placement policies can be imported using `<database>.<table>`.

```shell
terraform import mysql_ti_table_placement_policy.orders my_app.orders
```
