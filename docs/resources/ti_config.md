---
layout: "mysql"
page_title: "MySQL: mysql_ti_config"
sidebar_current: "docs-mysql-resource-ti-config"
description: |-
  Manages TiDB cluster configuration variables for PD and TiKV components.
---

# mysql\_ti\_config

The `mysql_ti_config` resource manages TiDB cluster configuration variables for PD and TiKV components using the `SET CONFIG` SQL statement.

~> **Note:** This resource is only compatible with TiDB clusters. It will not work with standard MySQL or MariaDB.

~> **Note on `destroy`:** When destroyed, this resource attempts to restore the configuration variable to its default value. Variables without known defaults are simply removed from state. See the [TiKV configuration reference](https://docs.pingcap.com/tidb/stable/tikv-configuration-file) and [PD configuration reference](https://docs.pingcap.com/tidb/stable/pd-configuration-file) for default values.

## Example Usage

### Setting a PD configuration variable

```hcl
resource "mysql_ti_config" "pd_log_level" {
  name  = "log.level"
  type  = "pd"
  value = "warn"
}
```

### Setting a TiKV configuration variable

```hcl
resource "mysql_ti_config" "tikv_raft_entry_max_size" {
  name  = "raftstore.raft-entry-max-size"
  type  = "tikv"
  value = "16MB"
}
```

### Setting a configuration variable on a specific instance

```hcl
resource "mysql_ti_config" "tikv_instance_gc" {
  name     = "gc.ratio-threshold"
  type     = "tikv"
  value    = "1"
  instance = "tikv-0:20160"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required, ForceNew) The name of the configuration variable (e.g., `log.level`, `raftstore.raft-entry-max-size`). Changing this forces a new resource.
* `type` - (Required, ForceNew) The TiDB component type. Must be one of `pd` or `tikv`. Changing this forces a new resource.
* `value` - (Required) The value to set for the configuration variable. Must not contain single quotes or be wrapped in backticks.
* `instance` - (Optional) A specific instance address (e.g., `tikv-0:20160`) to target. When omitted, the configuration is applied to all instances of the given `type`.

## Attributes Reference

* `id` - The ID of the resource, composed as `<type>#<name>` or `<type>#<name>#<instance>` when an instance is specified.

## Import

TiDB config variables can be imported using their ID format.

```shell
# Without instance
terraform import mysql_ti_config.example pd#log.level

# With instance
terraform import mysql_ti_config.example tikv#gc.ratio-threshold#tikv-0:20160
```
