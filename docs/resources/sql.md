---
layout: "mysql"
page_title: "MySQL: mysql_sql"
sidebar_current: "docs-mysql-resource-sql"
description: |-
  Executes arbitrary SQL statements for resource creation and deletion.
---

# mysql\_sql

The `mysql_sql` resource executes arbitrary SQL statements during Terraform's create and destroy lifecycle phases. This is useful for operations not covered by the provider's dedicated resources.

~> **Caution:** This resource executes raw SQL. Ensure your statements are idempotent and safe. The resource does **not** track state changes — the read operation is a no-op. If the underlying state changes outside of Terraform, it will not be detected.

~> **Note:** All arguments are `ForceNew` — any change to `name`, `create_sql`, or `delete_sql` will destroy and recreate the resource.

## Example Usage

### Creating a stored procedure

```hcl
resource "mysql_sql" "cleanup_proc" {
  name       = "cleanup_proc"
  create_sql = "CREATE PROCEDURE cleanup_old_records() BEGIN DELETE FROM logs WHERE created_at < DATE_SUB(NOW(), INTERVAL 90 DAY); END"
  delete_sql = "DROP PROCEDURE IF EXISTS cleanup_old_records"
}
```

### Creating an event

```hcl
resource "mysql_sql" "nightly_cleanup" {
  name       = "nightly_cleanup"
  create_sql = "CREATE EVENT IF NOT EXISTS nightly_cleanup ON SCHEDULE EVERY 1 DAY DO CALL cleanup_old_records()"
  delete_sql = "DROP EVENT IF EXISTS nightly_cleanup"
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required, ForceNew) A unique name to identify this resource in Terraform state. Used as the resource ID.
* `create_sql` - (Required, ForceNew) The SQL statement to execute when the resource is created.
* `delete_sql` - (Required, ForceNew) The SQL statement to execute when the resource is destroyed.

## Attributes Reference

* `id` - The value of the `name` argument.
