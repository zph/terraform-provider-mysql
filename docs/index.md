---
layout: "mysql"
page_title: "Provider: MySQL"
sidebar_current: "docs-mysql-index"
description: |-
  A provider for MySQL Server.
---

# MySQL Provider

[MySQL](http://www.mysql.com) is a relational database server. The MySQL
provider exposes resources used to manage the configuration of resources
in a MySQL server.

Use the navigation to the left to read about the available resources.

## Example Usage

The following is a minimal example:

```hcl
# Configure the MySQL provider
provider "mysql" {
  endpoint = "my-database.example.com:3306"
  username = "app-user"
  password = "app-password"
}

# Create a Database
resource "mysql_database" "app" {
  name = "my_awesome_app"
}
```

This provider can be used in conjunction with other resources that create
MySQL servers. For example, ``aws_db_instance`` is able to create MySQL
servers in Amazon's RDS service.

```hcl
# Create a database server
resource "aws_db_instance" "default" {
  engine         = "mysql"
  engine_version = "5.6.17"
  instance_class = "db.t1.micro"
  name           = "initial_db"
  username       = "rootuser"
  password       = "rootpasswd"

  # etc, etc; see aws_db_instance docs for more
}

# Configure the MySQL provider based on the outcome of
# creating the aws_db_instance.
provider "mysql" {
  endpoint = "${aws_db_instance.default.endpoint}"
  username = "${aws_db_instance.default.username}"
  password = "${aws_db_instance.default.password}"
}

# Create a second database, in addition to the "initial_db" created
# by the aws_db_instance resource above.
resource "mysql_database" "app" {
  name = "another_db"
}
```

Using encrypted connections can be done by using the `custom_tls` field in the provider

```hcl
provider "mysql" {
  endpoint = "my-database.example.com:3306"
  username = "app-user"
  custom_tls {
    config_key  = "custom_key"
    ca_cert     = "/path/to/certs/ca.pem"
    client_cert = "/path/to/certs/client_cert.pem"
    client_key  = "/path/to/certs/client_key.pem"
  }
}
```

And via Variables:

```hcl
variable "mysql_tls_ca_cert" {
  sensitive = true
  type = string
}
variable "mysql_tls_client_cert" {
  sensitive = true
  type = string
}
variable "mysql_tls_client_key" {
  sensitive = true
  type = string
}

provider "mysql" {
  endpoint = "my-database.example.com:3306"
  username = "app-user"
  custom_tls {
    config_key  = "custom_key"
    ca_cert     = var.mysql_tls_ca_cert
    client_cert = var.mysql_tls_client_cert
    client_key  = var.mysql_tls_client_key
  }
}
```

**Note** It it is _strongly_ recommended to ensure that these values/variables are marked as sensitive




### GCP CloudSQL Connection

For connections to GCP hosted instances, the provider can connect through the Cloud SQL MySQL library.

To enable Cloud SQL MySQL library, add `cloudsql://` to the endpoint `Network type` DSN string and connection name of the instance in following format: `project/region/instance` (or `project:region:instance`).

```hcl
# Configure the MySQL provider for CloudSQL Mysql
provider "mysql" {
  endpoint = "cloudsql://project:region:instance"
  username = "app-user"
  password = "app-password"
}
```

For a connection to an instance with no public IP add the `private_ip` option to the provider configuration.

```hcl
# Configure the MySQL provider for CloudSQL Mysql
provider "mysql" {
  endpoint = "cloudsql://project:region:instance"
  username = "app-user"
  password = "app-password"
  private_ip = true
}
```

See also: [Authentication at Google](https://cloud.google.com/docs/authentication#service-accounts).

### Azure MySQL server with AzureAD auth enabled connection

To use this authentication, add `azure://` to the  endpoint. This will lead to ignore `password` field which would be replaced by Azure AD
token of currently obtained identity. You have to use `username` as stated in Azure documentation.

```hcl
# Configure the MySQL provider for Azure Mysql Server with AzureAD authentication enabled
provider "mysql" {
  endpoint = "azure://your-azure-instance-name.mysql.database.azure.com"
  username = "username@yourtenant.onmicrosoft.com"
  # or if you granted access to AAD group: username = "Active_Directory_GroupName"
}
```

By default the provider will connect using DefaultAzureCredential from the Azure SDK for Go. The credentials can be provided by setting the `AZURE_*` environment variables, using a workload identity or a managed identity present on the host.

You can also further configure the Azure connection using the `azure_config` block:

```hcl
# Configure the MySQL provider for Azure Mysql Server with specific credentials
provider "mysql" {
  endpoint = "azure://your-azure-instance-name.mysql.database.azure.com"
  username = "username@yourtenant.onmicrosoft.com"

  azure_config {
    tenant_id     = "your-tenant-id"
    client_id     = "your-client-id"
    client_secret = var.client_secret
  }
}
```

See also: [Azure Active Directory authentication for MySQL](https://learn.microsoft.com/en-us/azure/mysql/flexible-server/how-to-azure-ad).

## SOCKS5 Proxy Support

The MySQL provider respects the `ALL_PROXY` and/or `all_proxy` environment variables.

```
$ export all_proxy="socks5://your.proxy:3306"
```

## Argument Reference

The following arguments are supported:

* `endpoint` - (Required) The address of the MySQL server to use. Most often a "hostname:port" pair, but may also be an absolute path to a Unix socket when the host OS is Unix-compatible. Can also be sourced from the `MYSQL_ENDPOINT` environment variable.
* `username` - (Required) Username to use to authenticate with the server, can also be sourced from the `MYSQL_USERNAME` environment variable.
* `password` - (Optional) Password for the given user, if that user has a password, can also be sourced from the `MYSQL_PASSWORD` environment variable.
* `proxy` - (Optional) Proxy socks url, can also be sourced from `ALL_PROXY` or `all_proxy` environment variables.
* `tls` - (Optional) The TLS configuration. One of `false`, `true`, or `skip-verify`. Defaults to `false`. Can also be sourced from the `MYSQL_TLS_CONFIG` environment variable.
* `custom_tls` - (Optional) Sets custom tls options for the connection. Documentation for encrypted connections can be found [here](https://dev.mysql.com/doc/refman/8.0/en/using-encrypted-connections.html). Consider setting shorter `connect_retry_timeout_sec` for debugging, as the default is 10 minutes .This is a block containing an optional `config_key`, which value is discarded but might be useful when troubleshooting, and the following required arguments:
  * `ca_cert` - Local filesystem path or string containing Certificate - If value begins with `-----BEGIN` we assume you're passing the certificate directly, otherwise a file from the local filesystem will be used.
  * `client_cert` - Local filesystem path or string containing Certificate - If value begins with `-----BEGIN` we assume you're passing the certificate directly, otherwise a file from the local filesystem will be used.
  * `client_key` - Local filesystem path or string containing Certificate - If value begins with `-----BEGIN` we assume you're passing the certificate directly, otherwise a file from the local filesystem will be used.

* `max_conn_lifetime_sec` - (Optional) Sets the maximum amount of time a connection may be reused. If d <= 0, connections are reused forever.
* `max_open_conns` - (Optional) Sets the maximum number of open connections to the database. If n <= 0, then there is no limit on the number of open connections.
* `conn_params` - (Optional) Sets extra mysql connection parameters (ODBC parameters). Most useful for session variables such as `default_storage_engine`, `foreign_key_checks` or `sql_log_bin`.
* `authentication_plugin` - (Optional) Sets the authentication plugin, it can be one of the following: `native` or `cleartext`. Defaults to `native`.
* `iam_database_authentication` - (Optional) For Cloud SQL databases, it enabled the use of IAM authentication. Make sure to declare the `password` field with a temporary OAuth2 token of the user that will connect to the MySQL server.
* `private_ip` - (Optional) Whether to use a connection to an instance with a private ip. Defaults to `false`. This argument only applies to CloudSQL and is ignored elsewhere.
* `azure_config` - (Optional) Sets the Azure configuration for the connection. This is a block containing the following arguments:
  * `client_id` - (Optional) The client ID for the Azure AD application. Can also be sourced from the `AZURE_CLIENT_ID` or `ARM_CLIENT_ID` environment variables.
  * `client_secret` - (Optional) The client secret for the Azure AD application. Can also be sourced from the `AZURE_CLIENT_SECRET` or `ARM_CLIENT_SECRET` environment variables.
  * `tenant_id` - (Optional) The tenant ID for the Azure AD application. Can also be sourced from the `AZURE_TENANT_ID` or `ARM_TENANT_ID` environment variables.
  * `environment` - (Optional) The Azure environment to use. Can also be sourced from the `AZURE_ENVIRONMENT` or `ARM_ENVIRONMENT` environment variables. Possible values are `public`, `china`, `german`, `usgovernment`. Defaults to `public`.
