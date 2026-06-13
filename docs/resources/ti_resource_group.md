---
layout: "mysql"
page_title: "MySQL: mysql_ti_resource_group"
sidebar_current: "docs-mysql-resource-ti-resource-group"
description: |-
  Creates and manages a TiDB resource group for workload isolation and prioritization.
---

# mysql\_ti\_resource\_group

The `mysql_ti_resource_group` resource creates and manages [TiDB resource groups](https://docs.pingcap.com/tidb/stable/tidb-resource-control-ru-groups/) for workload isolation and prioritization using Request Units (RU). The SQL syntax follows TiDB's [`CREATE RESOURCE GROUP`](https://docs.pingcap.com/tidb/stable/sql-statement-create-resource-group/) and [`ALTER RESOURCE GROUP`](https://docs.pingcap.com/tidb/stable/sql-statement-alter-resource-group/) statements.

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

### Resource group with TiDB v9 burstable mode

```hcl
resource "mysql_ti_resource_group" "interactive" {
  name           = "interactive"
  resource_units = 1000
  priority       = "high"
  burstable_mode = "unlimited"
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

### Default resource group background task limits

TiDB only supports `BACKGROUND` on the `default` resource group. See [TiDB background task resource control](https://docs.pingcap.com/tidb/stable/tidb-resource-control-background-tasks/).

```hcl
resource "mysql_ti_resource_group" "default" {
  name           = "default"
  resource_units = 2147483647
  burstable_mode = "unlimited"
  background     = "TASK_TYPES='br,ddl', UTILIZATION_LIMIT=30"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required, ForceNew) The name of the resource group. Changing this forces a new resource.
* `resource_units` - (Required) The number of Request Units per second (RU_PER_SEC) allocated to this resource group. This controls the throughput capacity. When reading existing TiDB resource groups, the provider maps TiDB's `UNLIMITED` value to `2147483647`.
* `priority` - (Optional) The priority level of the resource group. Must be one of `high`, `medium`, or `low`. Defaults to `medium`.
* `burstable` - (Optional) Legacy boolean form for whether the resource group can burst beyond its allocated RU_PER_SEC when there is spare capacity. Defaults to `false`.
* `burstable_mode` - (Optional, Computed) TiDB v9.0+ burst mode. Must be one of `off`, `moderated`, or `unlimited`. When set, this takes precedence over `burstable`.
* `query_limit` - (Optional) Runaway query control settings. When set, queries matching the criteria are automatically handled. This is the body of TiDB's `QUERY_LIMIT=(...)` clause. Supported TiDB criteria include `EXEC_ELAPSED='<duration>'`, `PROCESSED_KEYS=<count>`, and `RU=<count>`. Supported actions include `DRYRUN`, `COOLDOWN`, `KILL`, and, in TiDB v8.4.0 and later including v8.5.x, `SWITCH_GROUP(<resource_group>)`. The `WATCH` clause can use `EXACT`, `SIMILAR`, or `PLAN` with an optional `DURATION='<duration>'`. When empty (default), no query limit is applied.
* `background` - (Optional, Computed) Raw body for TiDB's `BACKGROUND=(...)` resource group clause. TiDB currently supports this only on the `default` resource group. Use `NULL` to emit `BACKGROUND=NULL`.

## Attributes Reference

* `id` - The name of the resource group.

## Import

Resource groups can be imported using their name.

```shell
terraform import mysql_ti_resource_group.example analytics
```
