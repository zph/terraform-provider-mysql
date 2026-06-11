---
layout: "mysql"
page_title: "MySQL: mysql_user_password"
sidebar_current: "docs-mysql-resource-user-password"
description: |-
  Creates and manages the password for a user on a MySQL server.
---
# mysql_user_password

The `mysql_user_password` resource sets and manages a password for a given
user on a MySQL server.

~> **NOTE on MySQL Passwords:** This resource should not be used together with
   password arguments on `mysql_user` for the same account.

~> **NOTE on How Passwords are Created:** When `plaintext_password` is omitted,
   this resource automatically generates a random UUID password and stores it
   in Terraform state. Provide `plaintext_password` yourself if you want to
   control the value.

## Example Usage

 ```hcl
resource "mysql_user" "jdoe" {
  user = "jdoe"
}

resource "mysql_user_password" "jdoe" {
  user    = mysql_user.jdoe.user
}
```

You can rotate passwords by running `terraform taint mysql_user_password.jdoe`.
The next time Terraform applies a new password will be generated and the user's
password will be updated accordingly.

## Argument Reference
The following arguments are supported:

* `user` - (Required) The MySQL user whose password should be managed.
* `host` - (Optional) The source host of the user. Defaults to `localhost`.
* `plaintext_password` - (Optional) The password to set. When omitted, a random UUID is generated.
* `retain_old_password` - (Optional) When `true`, the old password is retained while setting a new password. Requires MySQL 8.0.14 or newer.

## Attributes Reference

The following additional attributes are exported:

* `id` - The user and host, composed as `user@host`.
