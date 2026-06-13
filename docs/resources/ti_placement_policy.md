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
