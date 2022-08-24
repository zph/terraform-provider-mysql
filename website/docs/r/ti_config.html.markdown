---
layout: "mysql"
page_title: "MySQL: mysql_ti_config"
sidebar_current: "docs-mysql-resource-ti-config-variable"
description: |-
  Manages a TiKV or PD variables on a TiDB cluster.
---

# mysql\_ti\_config

The ``mysql_ti_config`` resource manages a TiKV or PD variables on a TiDB cluster.

~> **Note on TiDB:** Possible TiKV or PD variables are available [here](https://docs.pingcap.com/tidb/stable/dynamic-config)

~> **Note about `destroy`:** `destroy` is trying restore default values as described [here](https://github.com/petoju/terraform-provider-mysql/blob/master/mysql/resource_ti_config_defaults.go).
  Unfortunately not every variable support this.

## Example Usage

### PD

```hcl
resource "mysql_ti_config" "log_level" {
  name = "log.level"
  value = "warn"
  type = "pd"
}
```

#### Set variable for all PD instances

```hcl
resource "mysql_ti_config" "log_level" {
  name = "log.level"
  value = "warn"
  type = "pd"
}
```

#### Set variable for one PD instance only

```hcl
resource "mysql_ti_config" "log_level" {
  name = "log.level"
  value = "warn"
  type = "pd"
  instance = "127.0.0.1:2379"
}
```

## TiKV

### Set varibale for all TiKV instances

```hcl
resource "mysql_ti_config" "split_qps_threshold" {
  name = "split.qps-threshold"
  value = "100"
  type = "tikv"
}
```

#### Set variable for one TiKV instance only

```hcl
resource "mysql_ti_config" "split_qps_threshold" {
  name = "split.qps-threshold"
  value = "10"
  type = "tikv"
  instance = "127.0.0.1:20180"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required) The name of the configuration variable.
* `value` - (Required) The value of the configuration variable as string.
* `type` - (Required) The instance type to configure. Possible values are tikv or pd.

## Attributes Reference

No further attributes are exported.

## Import

TiKV or PD variable can be imported using global variable name.

General template to import is

```terraform import mysql_ti_config.<tf_name> <pd|tikv#config#optional_instance_name>```
```terraform import mysql_ti_config.<tf_config_name_in_tf_file> <pd|tikv#config_param_to_read#optional_instance_name>```

### Simple example

#### TiKV example

```shell
terraform import 'mysql_ti_config.split_qps_threshold' 'tikv#split-qps-threshold'
```

Import value for specific instance

```shell
terraform import 'mysql_ti_config.split_qps_threshold' 'tikv#split-qps-threshold#127.0.0.1:20180'
```

#### PD example

```shell
terraform import 'mysql_ti_config.log_level' 'pd#log.level'
```
