---
layout: "mysql"
page_title: "MySQL: mysql_ti_partition_placement_policy"
sidebar_current: "docs-mysql-resource-ti-partition-placement-policy"
description: |-
  Applies a TiDB placement policy to an existing table partition.
---

# mysql\_ti\_partition\_placement\_policy

The `mysql_ti_partition_placement_policy` resource applies a TiDB placement policy to an existing table partition using [`ALTER TABLE ... PARTITION ... PLACEMENT POLICY`](https://docs.pingcap.com/tidb/stable/placement-rules-in-sql/).

~> **Note:** This resource manages only the placement policy attachment. Destroying it resets the partition placement policy to TiDB's `default` policy; it does not drop the partition or table.

## TiDB Semantics

This resource is an attachment workaround for TiDB's placement DDL. TiDB does not provide a separate partition-placement attachment object, so the provider manages the direct assignment with `ALTER TABLE ... PARTITION ... PLACEMENT POLICY`.

On destroy, the provider runs `ALTER TABLE ... PARTITION ... PLACEMENT POLICY=default`. This removes the explicit partition placement policy and does not restore any previously configured policy. After reset, TiDB can apply placement inherited from the table, database, range, or global default.

Readback uses `information_schema.partitions.TIDB_PLACEMENT_POLICY_NAME`. A `NULL` value is represented as `default` in Terraform state, meaning there is no direct partition policy assignment. It does not necessarily mean the partition has no effective placement policy.

## Example Usage

```hcl
resource "mysql_ti_placement_policy" "history" {
  name        = "history"
  constraints = ["+node=history"]
}

resource "mysql_ti_partition_placement_policy" "orders_p0" {
  database         = "my_app"
  table            = "orders"
  partition        = "p0"
  placement_policy = mysql_ti_placement_policy.history.name
}
```

## Argument Reference

The following arguments are supported:

* `database` - (Required, ForceNew) Database containing the partitioned table.
* `table` - (Required, ForceNew) Partitioned table to configure.
* `partition` - (Required, ForceNew) Partition to configure.
* `placement_policy` - (Required) Placement policy to apply. Use `default` to reset the explicit partition placement policy.

## Attributes Reference

* `id` - The partition placement attachment ID, formatted as `<database>.<table>.<partition>`.

## Import

Partition placement policies can be imported using `<database>.<table>.<partition>`. If a database, table, or partition name contains a literal `.`, escape it as `\.`. If it contains a literal `\`, escape it as `\\`.

```shell
terraform import mysql_ti_partition_placement_policy.orders_p0 my_app.orders.p0
terraform import mysql_ti_partition_placement_policy.orders_archive_p0 'my_app.orders\.archive.p0'
```
