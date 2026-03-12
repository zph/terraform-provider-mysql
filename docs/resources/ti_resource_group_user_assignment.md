---
layout: "mysql"
page_title: "MySQL: mysql_ti_resource_group_user_assignment"
sidebar_current: "docs-mysql-resource-ti-resource-group-user-assignment"
description: |-
  Assigns a TiDB user to a resource group for workload isolation.
---

# mysql\_ti\_resource\_group\_user\_assignment

The `mysql_ti_resource_group_user_assignment` resource assigns a MySQL/TiDB user to a [TiDB resource group](https://docs.pingcap.com/tidb/stable/tidb-resource-control). This controls which resource group governs the user's query execution.

~> **Note:** This resource requires TiDB v7.5.0 or later.

~> **Note:** The user must already exist before assigning them to a resource group. Use `mysql_user` to create the user first.

~> **Note on `destroy`:** When destroyed, the user is reassigned to the `default` resource group.

## Example Usage

```hcl
resource "mysql_user" "analytics_user" {
  user               = "analytics_user"
  host               = "%"
  plaintext_password = "password"
}

resource "mysql_ti_resource_group" "analytics" {
  name           = "analytics"
  resource_units = 1000
  priority       = "low"
  burstable      = true
}

resource "mysql_ti_resource_group_user_assignment" "analytics_user" {
  user           = mysql_user.analytics_user.user
  resource_group = mysql_ti_resource_group.analytics.name
}
```

## Argument Reference

The following arguments are supported:

* `user` - (Required, ForceNew) The name of the user to assign to the resource group. The user must already exist. Changing this forces a new resource.
* `resource_group` - (Required) The name of the resource group to assign the user to. Can be updated to reassign the user to a different group.

## Attributes Reference

* `id` - The name of the assigned user.

## Import

User resource group assignments can be imported using the username.

```shell
terraform import mysql_ti_resource_group_user_assignment.example analytics_user
```
