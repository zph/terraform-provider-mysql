---
layout: "mysql"
page_title: "MySQL: mysql_ti_placement_range_policy"
sidebar_current: "docs-mysql-resource-ti-placement-range-policy"
description: |-
  Applies a TiDB placement policy to the global or metadata range.
---

# mysql\_ti\_placement\_range\_policy

The `mysql_ti_placement_range_policy` resource applies a TiDB placement policy to the `global` or `meta` range using [`ALTER RANGE`](https://docs.pingcap.com/tidb/stable/sql-statement-alter-range/).

~> **Note:** TiDB readback uses [`SHOW PLACEMENT`](https://docs.pingcap.com/tidb/stable/placement-rules-in-sql/), which returns the expanded placement and scheduling state for the range. It does not return the original placement policy name, so this resource cannot fully detect out-of-band changes to the assigned policy name.

## Example Usage

```hcl
resource "mysql_ti_placement_policy" "five_replicas" {
  name      = "five_replicas"
  followers = 4
}

resource "mysql_ti_placement_range_policy" "global" {
  range            = "global"
  placement_policy = mysql_ti_placement_policy.five_replicas.name
}
```

## Argument Reference

The following arguments are supported:

* `range` - (Required, ForceNew) TiDB range to configure. Must be `global` or `meta`.
* `placement_policy` - (Required) Placement policy to apply to the range.

## Attributes Reference

* `placement` - Expanded placement text returned by `SHOW PLACEMENT`.
* `scheduling_state` - Placement scheduling state returned by `SHOW PLACEMENT`.

## Import

Range placement policies can be imported using the range name.

```shell
terraform import mysql_ti_placement_range_policy.global global
```
