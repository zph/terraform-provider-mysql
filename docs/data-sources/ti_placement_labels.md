---
layout: "mysql"
page_title: "MySQL: mysql_ti_placement_labels"
sidebar_current: "docs-mysql-datasource-ti-placement-labels"
description: |-
  Lists TiDB placement labels available in the cluster.
---

# mysql\_ti\_placement\_labels

The `mysql_ti_placement_labels` data source reads TiDB placement labels using [`SHOW PLACEMENT LABELS`](https://docs.pingcap.com/tidb/stable/sql-statement-show-placement-labels/). These labels are useful when building TiDB placement policies.

## Example Usage

```hcl
data "mysql_ti_placement_labels" "available" {}
```

## Attributes Reference

* `labels` - List of placement labels.

Each `labels` item contains:

* `key` - Placement label key, such as `region` or `zone`.
* `values` - Available values for the label.
