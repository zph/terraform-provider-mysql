# Implementation Guide

## Overview

`terraform-provider-mysql` is a Terraform provider for managing MySQL and TiDB server resources. Built with the Terraform Plugin SDK v2.

## Architecture

```
main.go                          # Entry point, registers provider
mysql/
  provider.go                    # Provider schema (endpoint, auth, TLS, cloud connectors)
  resource_*.go                  # Resource implementations (CRUD)
  data_source_*.go               # Data source implementations (read-only)
  utils.go                       # Shared helpers (DB connection, error handling)
  testcontainers_helper.go       # Test infrastructure
  provider_test_common.go        # Shared test fixtures
docs/
  index.md                       # Provider documentation
  resources/*.md                 # Resource documentation
  data-sources/*.md              # Data source documentation
scripts/
  make-release.go                # Release automation
  test-runner.go                 # Test runner script
```

## Resources

### MySQL Resources
| Resource | File | Description |
|----------|------|-------------|
| `mysql_database` | `resource_database.go` | Database CRUD |
| `mysql_user` | `resource_user.go` | User management (password, auth plugins, AAD) |
| `mysql_user_password` | `resource_user_password.go` | Auto-generated password management with PGP |
| `mysql_grant` | `resource_grant.go` | Privilege and role grants |
| `mysql_role` | `resource_role.go` | Role CRUD (MySQL 8+) |
| `mysql_default_roles` | `resource_default_roles.go` | Default role assignment (MySQL 8+) |
| `mysql_global_variable` | `resource_global_variable.go` | Global variable management |
| `mysql_sql` | `resource_sql.go` | Arbitrary SQL execution |
| `mysql_rds_config` | `resource_rds_config.go` | AWS RDS-specific config (binlog retention, replication delay) |

### TiDB Resources
| Resource | File | Description |
|----------|------|-------------|
| `mysql_ti_config` | `resource_ti_config_variable.go` | PD/TiKV config via `SET CONFIG` |
| `mysql_ti_resource_group` | `resource_ti_resource_group.go` | Resource group management (v7.5+) |
| `mysql_ti_resource_group_user_assignment` | `resource_ti_resource_group_user_assignment.go` | User-to-resource-group binding |
| `mysql_ti_placement_policy` | `resource_ti_placement_policy.go` | Placement policies for data distribution |

### Data Sources
| Data Source | File | Description |
|-------------|------|-------------|
| `mysql_databases` | `data_source_databases.go` | List databases (with LIKE filter) |
| `mysql_tables` | `data_source_tables.go` | List tables in a database (with LIKE filter) |

## Provider Configuration

Supports multiple connection modes:
- **Direct TCP/Unix socket** — standard MySQL connections
- **GCP CloudSQL** — via `cloudsql://` endpoint prefix, supports IAM auth
- **Azure MySQL** — via `azure://` endpoint prefix, Azure AD token auth
- **SOCKS5 proxy** — via `proxy` arg or `ALL_PROXY` env var
- **Custom TLS** — certificates via file paths or inline PEM strings

## TiDB Implementation Details

### Config Variables (`ti_config`)
- Uses `SET CONFIG <type> <name>=<value>` SQL syntax
- On destroy, restores defaults from `resource_ti_config_defaults.go` struct
- Variables marked `IGNOREONDESTROY#` are removed from state only (no restore)
- ID format: `<type>#<name>` or `<type>#<name>#<instance>`

### Resource Groups (`ti_resource_group`)
- Requires TiDB v7.5.0+
- Uses `CREATE/ALTER/DROP RESOURCE GROUP` SQL
- Supports RU_PER_SEC, priority, burstable, and query limits (runaway queries)

### Placement Policies (`ti_placement_policy`)
- Uses `CREATE/ALTER/DROP PLACEMENT POLICY` SQL
- Supports primary_region, regions list, and constraints
- Reads from `information_schema.placement_policies`

## Testing

- Uses testcontainers for integration tests (`testcontainers_helper.go`)
- Test files follow `resource_*_test.go` naming convention
- Run tests via `scripts/test-runner.go`
