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

~> **Note:** Destroying this resource resets the range placement policy to TiDB's `default` policy.

## TiDB Semantics

This resource is an attachment workaround for TiDB's range placement DDL. TiDB does not provide a separate range-placement attachment object, so the provider manages the direct assignment with `ALTER RANGE ... PLACEMENT POLICY`.

On destroy, the provider runs `ALTER RANGE ... PLACEMENT POLICY=default`. This resets the explicit range placement policy and does not restore any previously configured policy.

Range readback is weaker than database, table, and partition readback. TiDB's `SHOW PLACEMENT` returns `Target`, expanded `Placement`, and `Scheduling_State`, but not the original policy name assigned with `ALTER RANGE`. Because of that TiDB limitation, range import requires `<range>:<placement_policy>`, and this resource cannot fully detect an out-of-band change that switches to another policy with equivalent placement options.

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

Range placement policies can be imported using `<range>:<placement_policy>`. The placement policy name is required because TiDB does not expose the original policy name in range readback.

```shell
terraform import mysql_ti_placement_range_policy.global global:five_replicas
```
