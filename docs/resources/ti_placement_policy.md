---
layout: "mysql"
page_title: "MySQL: mysql_ti_placement_policy"
sidebar_current: "docs-mysql-resource-ti-placement-policy"
description: |-
  Creates and manages a TiDB placement policy for controlling data placement across regions.
---

# mysql\_ti\_placement\_policy

The `mysql_ti_placement_policy` resource creates and manages [TiDB placement policies](https://docs.pingcap.com/tidb/stable/placement-rules-in-sql) which control how data is distributed across TiKV nodes, regions, and availability zones.

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

### Using a placement policy with a database

```hcl
resource "mysql_ti_placement_policy" "regional" {
  name           = "regional"
  primary_region = "us-east-1"
  regions        = ["us-east-1", "us-west-2"]
}

resource "mysql_database" "app" {
  name = "my_app"
  # Reference the placement policy in TiDB SQL:
  # ALTER DATABASE my_app PLACEMENT POLICY = regional;
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required, ForceNew) The name of the placement policy. Changing this forces a new resource.
* `primary_region` - (Optional) The primary region for leader replicas. Leaders will preferentially be placed in this region.
* `regions` - (Optional) A list of regions where data replicas can be placed. Order may influence follower placement preference.
* `constraints` - (Optional) A list of label constraints that control replica placement. Uses the format `+key=value` to require a label or `-key=value` to prohibit it. When omitted or empty, no constraints are applied.

## Attributes Reference

* `id` - The name of the placement policy.

## Import

Placement policies can be imported using their name.

```shell
terraform import mysql_ti_placement_policy.example us_east
```
