---
layout: "mysql"
page_title: "MySQL: mysql_user"
sidebar_current: "docs-mysql-resource-user"
description: |-
  Creates and manages a user on a MySQL server.
---

# mysql\_user

The ``mysql_user`` resource creates and manages a user on a MySQL
server.

For TiDB-specific user syntax, this resource also supports options documented in TiDB's [`CREATE USER`](https://docs.pingcap.com/tidb/stable/sql-statement-create-user/) and [`ALTER USER`](https://docs.pingcap.com/tidb/stable/sql-statement-alter-user/) statements, including resource group assignment and password lifecycle controls.

~> **Note:** The password for the user is provided in plain text, and is
obscured by an unsalted hash in the state
[Read more about sensitive data in state](https://www.terraform.io/language/state/sensitive-data).
Care is required when using this resource, to avoid disclosing the password.

## Example Usage

```hcl
resource "mysql_user" "jdoe" {
  user               = "jdoe"
  host               = "example.com"
  plaintext_password = "password"
}
```

## Example Usage with an Authentication Plugin

```hcl
resource "mysql_user" "nologin" {
  user               = "nologin"
  host               = "example.com"
  auth_plugin        = "mysql_no_login"
}
```

## Example Usage with an Authentication Plugin and hashed password

```hcl
resource "mysql_user" "nologin" {
  user               = "nologin"
  host               = "example.com"
  auth_plugin        = "mysql_native_password"
  auth_string_hashed = "*2470C0C06DEE42FD1618BB99005ADCA2EC9D1E19"
}
```

## Example Usage with AzureAD Authentication Plugin

```hcl
resource "mysql_user" "aadupn" {
  user = "alias_to_use_when_connecting"
  auth_plugin = "aad_auth"
  aad_identity {
    type = "user" # user | group | service_principal
    identity = "little.johny@doe.onmicrosoft.com" # upn | group name | client id of service principal
  }
}
```

## Example Usage with TiDB Resource Controls

```hcl
resource "mysql_user" "app" {
  user                    = "app"
  host                    = "%"
  plaintext_password      = "correct-horse-battery-staple"
  resource_group          = "app_rg"
  max_user_connections    = 20
  account_locked          = false
  comment                 = "application user"
  attribute_json          = jsonencode({ team = "platform" })
  password_expire         = "interval 90 day"
  password_history        = "5"
  password_reuse_interval = "30 day"
  failed_login_attempts   = 3
  password_lock_time      = "unbounded"
}
```

~> **Note on Azure Database for MySQL Single Server resource:** If you want to use this for `service_principal` with older Azure Database for MySQL Single Server resource, you need to set param `aad_auth_validate_oids_in_tenant` to `OFF` in provider configuration. For more details see [this issue](https://github.com/petoju/terraform-provider-mysql/issues/79).

## Example Usage with Resource Limits

```hcl
# MySQL, MariaDB, and TiDB 8.5.5+: set MAX_USER_CONNECTIONS
# (read back on MySQL/MariaDB, and on TiDB builds exposing the mysql.user column)
resource "mysql_user" "limited" {
  user                 = "app_user"
  host                 = "%"
  plaintext_password   = "password"
  max_user_connections = 100
}

# MariaDB only: set MAX_STATEMENT_TIME
resource "mysql_user" "limited_mariadb" {
  user                 = "app_user"
  host                 = "%"
  plaintext_password   = "password"
  max_user_connections = 100
  max_statement_time   = 30.0
}
```

## Argument Reference

The following arguments are supported:

* `user` - (Required) The name of the user.
* `host` - (Optional) The source host of the user. Defaults to "localhost".
* `plaintext_password` - (Optional) The password for the user. This must be provided in plain text, so the data source for it must be secured. An _unsalted_ hash of the provided password is stored in state. Conflicts with `auth_plugin`.
* `password` - (Optional) Deprecated alias of `plaintext_password`, whose value is *stored as plaintext in state*. Prefer to use `plaintext_password` instead, which stores the password as an unsalted hash. Conflicts with `auth_plugin`.
* `auth_plugin` - (Optional) Use an [authentication plugin][ref-auth-plugins] to authenticate the user instead of using password authentication.  Description of the fields allowed in the block below. Conflicts with `password` and `plaintext_password`.  
* `auth_string_hashed` - (Optional) Use an already hashed string as a parameter to `auth_plugin`. This can be used with passwords as well as with other auth strings.
* `aad_identity` - (Optional) Required when `auth_plugin` is `aad_auth`. This should be block containing `type` and `identity`. `type` can be one of `user`, `group` and `service_principal`. `identity` then should containt either UPN of user, name of group or Client ID of service principal.
* `retain_old_password` - (Optional) When `true`, the old password is retained when changing the password. Defaults to `false`. This use MySQL Dual Password Support feature and requires MySQL version 8.0.14 or newer. See [MySQL Dual Password documentation](https://dev.mysql.com/doc/refman/8.0/en/password-management.html#dual-passwords) for more.
* `tls_option` - (Optional) An TLS-Option for the `CREATE USER` or `ALTER USER` statement. The value is suffixed to `REQUIRE`. A value of 'SSL' will generate a `CREATE USER ... REQUIRE SSL` statement. See the [MYSQL `CREATE USER` documentation](https://dev.mysql.com/doc/refman/5.7/en/create-user.html) for more. Ignored if MySQL version is under 5.7.0.
* `resource_group` - (Optional, Computed, TiDB) Assigns the user to a TiDB resource group using `CREATE/ALTER USER ... RESOURCE GROUP`.
* `max_user_connections` - (Optional) Maximum number of simultaneous connections for the user. A value of `0` means unlimited. On MySQL and MariaDB the value is persisted in `mysql.user` and read back for drift detection. On TiDB the clause is accepted only on 8.5.5 or newer (older versions are rejected by the provider). TiDB never echoes the value in `SHOW CREATE USER`; instead, builds carrying [pingcap/tidb#59197](https://github.com/pingcap/tidb/pull/59197) persist it in a `mysql.user.max_user_connections` column. The provider detects that column at runtime — not all 8.5.x builds carry it (for example the upstream `pingcap/tidb:v8.5.6` image does not) — and reads the value back for drift detection when present, otherwise applies it best-effort and write-only. When this argument is removed from configuration, the limit is reset to `0`.
* `max_statement_time` - (Optional) Maximum execution time for statements in seconds. A value of `0` means unlimited. Supports fractional values for subsecond precision, for example `0.01` for 10 milliseconds. Only supported on MariaDB 10.1.1 or newer; MySQL and TiDB do not support this account option.
* `account_locked` - (Optional, Computed) Sets `ACCOUNT LOCK` or `ACCOUNT UNLOCK`.
* `comment` - (Optional, Computed, TiDB) Sets the TiDB user `COMMENT` value.
* `attribute_json` - (Optional, Computed, TiDB) Sets the TiDB user `ATTRIBUTE` JSON value.
* `password_expire` - (Optional, Computed) Raw value for `PASSWORD EXPIRE`, such as `default`, `never`, or `interval 90 day`.
* `password_history` - (Optional, Computed) Raw value for `PASSWORD HISTORY`, such as `default` or `5`.
* `password_reuse_interval` - (Optional, Computed) Raw value for `PASSWORD REUSE INTERVAL`, such as `default` or `30 day`.
* `failed_login_attempts` - (Optional, Computed) Sets `FAILED_LOGIN_ATTEMPTS`.
* `password_lock_time` - (Optional, Computed) Raw value for `PASSWORD_LOCK_TIME`, such as `2` or `unbounded`.

[ref-auth-plugins]: https://dev.mysql.com/doc/refman/5.7/en/authentication-plugins.html

The `auth_plugin` value supports:

* `AWSAuthenticationPlugin` - Allows the use of IAM authentication with [Amazon
  Aurora][ref-amazon-aurora]. For more details on how to use IAM auth with
  Aurora, see [here][ref-aurora-using-iam].

[ref-amazon-aurora]: https://aws.amazon.com/rds/aurora/
[ref-aurora-using-iam]: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.html#UsingWithRDS.IAMDBAuth.Creating

* `mysql_no_login` - Uses the MySQL No-Login Authentication Plugin. The
  No-Login Authentication Plugin must be active in MySQL. For more information,
  see [here][ref-mysql-no-login].

[ref-mysql-no-login]: https://dev.mysql.com/doc/refman/5.7/en/no-login-pluggable-authentication.html

* `aad_auth` - Uses `CREATE AADUSER` statement to create user instead of `CREATE USER` to create user
   with [AzureAD authentication][ref-azure-aadauth] to [Azure Database for MySQL][ref-azure-mysql].
   When specified, you need to specify `aad_identity`. For more information about AzureAD authentication into MySQL  
   see [here][ref-azure-aadauth]. You have to use AAD authenticated administrator mysql session to use this plugin.

[ref-azure-aadauth]: https://learn.microsoft.com/en-us/azure/mysql/flexible-server/how-to-azure-ad
[ref-azure-mysql]: https://learn.microsoft.com/en-us/azure/mysql/

* any other auth plugin supported by MySQL.
## Attributes Reference

The following attributes are exported:

* `user` - The name of the user.
* `host` - The host where the user was created.
* `id` - The id of the user created, composed as "username@host".

## Import

Users can be imported using user and host.

```
$ terraform import mysql_user.example user@host
```
