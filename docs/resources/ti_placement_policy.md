---
layout: "mysql"
page_title: "MySQL: mysql_ti_placement_policy"
sidebar_current: "docs-mysql-resource-ti-placement-policy"
description: |-
  Creates and manages a TiDB placement policy for controlling data placement across regions.
---

# mysql\_ti\_placement\_policy

The `mysql_ti_placement_policy` resource creates and manages [TiDB placement policies](https://docs.pingcap.com/tidb/stable/placement-rules-in-sql) which control how data is distributed across TiKV nodes, regions, and availability zones. The SQL syntax follows TiDB's [`CREATE PLACEMENT POLICY`](https://docs.pingcap.com/tidb/stable/sql-statement-create-placement-policy/) and [`ALTER PLACEMENT POLICY`](https://docs.pingcap.com/tidb/stable/sql-statement-alter-placement-policy/) statements.

~> **Note:** This resource is only compatible with TiDB clusters. It will not work with standard MySQL or MariaDB.

## Example Usage

### Basic placement policy with primary region

```hcl
resource "mysql_ti_placement_policy" "us_east" {
  name           = "us_east"
  primary_region = "us-east-1"
  regions        = ["us-east-1", "us-east-2", "us-west-1"]
}
```

### Placement policy with constraints

```hcl
resource "mysql_ti_placement_policy" "ssd_only" {
  name        = "ssd_only"
  constraints = ["+disk=ssd"]
}
```

### Placement policy with replica and role-specific constraints

```hcl
resource "mysql_ti_placement_policy" "multi_region" {
  name                  = "multi_region"
  primary_region        = "us-east-1"
  regions               = ["us-east-1", "us-west-2"]
  followers             = 4
  learners              = 1
  schedule              = "EVEN"
  constraints           = ["+disk=ssd"]
  leader_constraints    = ["+zone=us-east-1a"]
  follower_constraints  = ["+zone=us-west-2a"]
  learner_constraints   = ["+zone=us-west-2b"]
  survival_preferences  = ["region", "zone"]
}
```

### Using a placement policy with a database

```hcl
resource "mysql_ti_placement_policy" "regional" {
  name           = "regional"
  primary_region = "us-east-1"
  regions        = ["us-east-1", "us-west-2"]
}

resource "mysql_database" "app" {
  name             = "my_app"
  placement_policy = mysql_ti_placement_policy.regional.name
}
```

### Applying a placement policy to existing objects

```hcl
resource "mysql_ti_table_placement_policy" "orders" {
  database         = "my_app"
  table            = "orders"
  placement_policy = mysql_ti_placement_policy.regional.name
}

resource "mysql_ti_partition_placement_policy" "orders_p0" {
  database         = "my_app"
  table            = "orders"
  partition        = "p0"
  placement_policy = mysql_ti_placement_policy.regional.name
}
```

## Provider Behavior and TiDB Readback Limitations

TiDB placement policies are attached to existing objects with DDL such as `ALTER DATABASE`, `ALTER TABLE`, `ALTER TABLE ... PARTITION`, and `ALTER RANGE`. TiDB does not expose a separate "placement attachment" object that Terraform can create or delete. For that reason, the provider models database, table, partition, and range placement assignment as attachment resources.

Destroying an attachment resource does not drop the underlying database, table, partition, or range. Instead, the provider runs the matching TiDB reset DDL with `PLACEMENT POLICY=default`. On TiDB this removes the explicit placement attachment for that scope. It does not restore any previous policy, and for tables or partitions it can cause the object to inherit placement from a broader scope such as a table, database, range, or global default.

Readback is also uneven across TiDB placement scopes:

* Databases, tables, and partitions expose direct policy names through `information_schema.schemata`, `information_schema.tables`, and `information_schema.partitions` as `TIDB_PLACEMENT_POLICY_NAME`.
* A `NULL` policy name means "no direct policy attached at this scope". It does not necessarily mean that the object has no effective placement policy, because TiDB can inherit placement from broader scopes.
* The provider maps `NULL` readback to `default` for attachment resources so Terraform has a stable value for "no direct attachment".
* Range placement is different: `SHOW PLACEMENT` returns the expanded placement and scheduling state for `RANGE TiDB_GLOBAL` and `RANGE TiDB_META`, but does not return the original policy name assigned with `ALTER RANGE`.
* Because range readback lacks the original policy name, range placement import requires `<range>:<placement_policy>`, and the provider cannot fully detect out-of-band changes that replace a range policy with another policy that expands to the same placement options.

These behaviors line up with TiDB's upstream history:

* Table and partition policy-name readback was added for machine-readable placement lookup in [pingcap/tidb#28798](https://github.com/pingcap/tidb/pull/28798), and schema-level readback was requested in [pingcap/tidb#29758](https://github.com/pingcap/tidb/issues/29758).
* `TIDB_DIRECT_PLACEMENT` was later removed from the relevant `information_schema` tables in [pingcap/tidb#31741](https://github.com/pingcap/tidb/pull/31741), so this provider intentionally relies only on `TIDB_PLACEMENT_POLICY_NAME`.
* Range placement has known version-specific defects and open issues, including missing permission checks for `ALTER RANGE` ([pingcap/tidb#62420](https://github.com/pingcap/tidb/issues/62420)) and `ALTER RANGE meta` RawKV range overlap ([pingcap/tidb#63133](https://github.com/pingcap/tidb/issues/63133), [pingcap/tidb#63236](https://github.com/pingcap/tidb/pull/63236)).
* Older TiDB versions also had fixed defects around `ALTER RANGE meta`, policy updates for ranges, dropping similarly named policies, partition placement DDL, and TiFlash compute placement. See [pingcap/tidb#60888](https://github.com/pingcap/tidb/issues/60888), [pingcap/tidb#51712](https://github.com/pingcap/tidb/issues/51712), [pingcap/tidb#52257](https://github.com/pingcap/tidb/issues/52257), [pingcap/tidb#48630](https://github.com/pingcap/tidb/issues/48630), and [pingcap/tidb#58633](https://github.com/pingcap/tidb/issues/58633).

## Argument Reference

The following arguments are supported:

* `name` - (Required, ForceNew) The name of the placement policy. Changing this forces a new resource.
* `primary_region` - (Optional) The primary region for leader replicas. Leaders will preferentially be placed in this region.
* `regions` - (Optional) A list of regions where data replicas can be placed. Order may influence follower placement preference.
* `followers` - (Optional) Number of follower replicas.
* `schedule` - (Optional) TiDB placement schedule strategy.
* `learners` - (Optional) Number of learner replicas.
* `constraints` - (Optional) A list of label constraints that control replica placement. Uses the format `+key=value` to require a label or `-key=value` to prohibit it. When omitted or empty, no constraints are applied.
* `leader_constraints` - (Optional) Label constraints for leader replicas.
* `follower_constraints` - (Optional) Label constraints for follower replicas.
* `learner_constraints` - (Optional) Label constraints for learner replicas.
* `survival_preferences` - (Optional) Ordered labels for TiDB's `SURVIVAL_PREFERENCES` clause. TiDB accepts this in `CREATE/ALTER PLACEMENT POLICY`, but the documented `information_schema.placement_policies` table does not expose it for drift detection.

## Attributes Reference

* `id` - The name of the placement policy.

## Import

Placement policies can be imported using their name.

```shell
terraform import mysql_ti_placement_policy.example us_east
```
