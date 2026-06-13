---
layout: "mysql"
page_title: "MySQL: mysql_ti_resource_group"
sidebar_current: "docs-mysql-resource-ti-resource-group"
description: |-
  Creates and manages a TiDB resource group for workload isolation and prioritization.
---

# mysql\_ti\_resource\_group

The `mysql_ti_resource_group` resource creates and manages [TiDB resource groups](https://docs.pingcap.com/tidb/stable/tidb-resource-control) for workload isolation and prioritization using Request Units (RU).

~> **Note:** This resource requires TiDB v7.5.0 or later.

~> **Note:** This resource is only compatible with TiDB clusters. It will not work with standard MySQL or MariaDB.

## Example Usage

### Basic resource group

```hcl
resource "mysql_ti_resource_group" "analytics" {
  name           = "analytics"
  resource_units = 1000
}
```

### Resource group with priority and burstable

```hcl
resource "mysql_ti_resource_group" "batch_jobs" {
  name           = "batch_jobs"
  resource_units = 500
  priority       = "low"
  burstable      = true
}
```

### Resource group with query limit (runaway query control)

```hcl
resource "mysql_ti_resource_group" "oltp" {
  name           = "oltp"
  resource_units = 2000
  priority       = "high"
  query_limit    = "EXEC_ELAPSED='60s', ACTION=KILL, WATCH=EXACT DURATION='10m'"
}
```

### Resource group with v8.5 runaway query controls

```hcl
resource "mysql_ti_resource_group" "oltp_guardrail" {
  name           = "oltp_guardrail"
  resource_units = 2000
  priority       = "high"
  query_limit    = "PROCESSED_KEYS=1000000, RU=2000, ACTION=SWITCH_GROUP(rg_quarantine), WATCH=PLAN DURATION='30m'"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required, ForceNew) The name of the resource group. Changing this forces a new resource.
* `resource_units` - (Required) The number of Request Units per second (RU_PER_SEC) allocated to this resource group. This controls the throughput capacity. When reading existing TiDB resource groups, the provider maps TiDB's `UNLIMITED` value to `2147483647`.
* `priority` - (Optional) The priority level of the resource group. Must be one of `high`, `medium`, or `low`. Defaults to `medium`.
* `burstable` - (Optional) Whether the resource group can burst beyond its allocated RU_PER_SEC when there is spare capacity. Defaults to `false`.
* `query_limit` - (Optional) Runaway query control settings. When set, queries matching the criteria are automatically handled. This is the body of TiDB's `QUERY_LIMIT=(...)` clause. Supported TiDB criteria include `EXEC_ELAPSED='<duration>'`, `PROCESSED_KEYS=<count>`, and `RU=<count>`. Supported actions include `DRYRUN`, `COOLDOWN`, `KILL`, and, in TiDB v8.4.0 and later including v8.5.x, `SWITCH_GROUP(<resource_group>)`. The `WATCH` clause can use `EXACT`, `SIMILAR`, or `PLAN` with an optional `DURATION='<duration>'`. When empty (default), no query limit is applied.

## Attributes Reference

* `id` - The name of the resource group.

## Import

Resource groups can be imported using their name.

```shell
terraform import mysql_ti_resource_group.example analytics
```
