package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const unknownTableErrCode = 1146
const unknownColumnErrCode = 1054

/*
TiDB exposes placement assignment as ALTER statements on existing objects rather
than as separate attachment objects. These Terraform resources are intentionally
thin "attachment" resources around that DDL so existing databases, tables, and
partitions can be managed without taking ownership of their lifecycle.

The delete semantics are similarly TiDB-specific: there is no DROP ATTACHMENT
statement. TiDB resets a placement assignment with PLACEMENT POLICY=default,
which removes the explicit direct policy at that scope. For tables and
partitions, that can reveal inherited placement from broader scopes. Readback
also reports only the direct assignment in TIDB_PLACEMENT_POLICY_NAME; NULL does
not mean "no effective placement", only "no direct placement policy here". The
provider normalizes NULL to "default" so Terraform has a stable representation
for the reset state.
*/
func resourceTiDatabasePlacementPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateOrUpdateTiDatabasePlacementPolicy,
		ReadContext:   ReadTiDatabasePlacementPolicy,
		UpdateContext: CreateOrUpdateTiDatabasePlacementPolicy,
		DeleteContext: DeleteTiDatabasePlacementPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: ImportTiDatabasePlacementPolicy,
		},
		Schema: map[string]*schema.Schema{
			"database": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"placement_policy": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceTiTablePlacementPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateOrUpdateTiTablePlacementPolicy,
		ReadContext:   ReadTiTablePlacementPolicy,
		UpdateContext: CreateOrUpdateTiTablePlacementPolicy,
		DeleteContext: DeleteTiTablePlacementPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: ImportTiTablePlacementPolicy,
		},
		Schema: map[string]*schema.Schema{
			"database": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"table": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"placement_policy": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceTiPartitionPlacementPolicy() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateOrUpdateTiPartitionPlacementPolicy,
		ReadContext:   ReadTiPartitionPlacementPolicy,
		UpdateContext: CreateOrUpdateTiPartitionPlacementPolicy,
		DeleteContext: DeleteTiPartitionPlacementPolicy,
		Importer: &schema.ResourceImporter{
			StateContext: ImportTiPartitionPlacementPolicy,
		},
		Schema: map[string]*schema.Schema{
			"database": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"table": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"partition": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"placement_policy": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func CreateOrUpdateTiDatabasePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "database placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database := d.Get("database").(string)
	placementPolicy := d.Get("placement_policy").(string)
	if err := setTiDatabasePlacementPolicy(ctx, db, database, placementPolicy); err != nil {
		return diag.Errorf("error setting TiDB placement policy (%s) on database (%s): %s", placementPolicy, database, err)
	}

	d.SetId(formatTiDatabasePlacementPolicyID(database))
	return ReadTiDatabasePlacementPolicy(ctx, d, meta)
}

func ReadTiDatabasePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "database placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database, err := tiDatabasePlacementPolicyFields(d)
	if err != nil {
		return diag.FromErr(err)
	}

	placementPolicy, exists, err := readTiDatabasePlacementPolicyFromDB(ctx, db, database)
	if err != nil {
		return diag.Errorf("error reading TiDB database placement policy (%s): %s", database, err)
	}
	if !exists {
		d.SetId("")
		return nil
	}

	d.Set("database", database)
	d.Set("placement_policy", placementPolicy)
	d.SetId(formatTiDatabasePlacementPolicyID(database))
	return nil
}

func DeleteTiDatabasePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "database placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database, err := tiDatabasePlacementPolicyFields(d)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := setTiDatabasePlacementPolicy(ctx, db, database, placementPolicyDefault); err != nil {
		if mysqlErrorNumber(err) == unknownDatabaseErrCode {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error resetting TiDB placement policy on database (%s): %s", database, err)
	}

	d.SetId("")
	return nil
}

func CreateOrUpdateTiTablePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "table placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database := d.Get("database").(string)
	table := d.Get("table").(string)
	placementPolicy := d.Get("placement_policy").(string)
	if err := setTiTablePlacementPolicy(ctx, db, database, table, placementPolicy); err != nil {
		return diag.Errorf("error setting TiDB placement policy (%s) on table (%s.%s): %s", placementPolicy, database, table, err)
	}

	d.SetId(formatTiTablePlacementPolicyID(database, table))
	return ReadTiTablePlacementPolicy(ctx, d, meta)
}

func ReadTiTablePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "table placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database, table, err := tiTablePlacementPolicyFields(d)
	if err != nil {
		return diag.FromErr(err)
	}

	placementPolicy, exists, err := readTiTablePlacementPolicyFromDB(ctx, db, database, table)
	if err != nil {
		return diag.Errorf("error reading TiDB table placement policy (%s.%s): %s", database, table, err)
	}
	if !exists {
		d.SetId("")
		return nil
	}

	d.Set("database", database)
	d.Set("table", table)
	d.Set("placement_policy", placementPolicy)
	d.SetId(formatTiTablePlacementPolicyID(database, table))
	return nil
}

func DeleteTiTablePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "table placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database, table, err := tiTablePlacementPolicyFields(d)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := setTiTablePlacementPolicy(ctx, db, database, table, placementPolicyDefault); err != nil {
		if mysqlErrorNumber(err) == unknownDatabaseErrCode || mysqlErrorNumber(err) == unknownTableErrCode {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error resetting TiDB placement policy on table (%s.%s): %s", database, table, err)
	}

	d.SetId("")
	return nil
}

func CreateOrUpdateTiPartitionPlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "partition placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database := d.Get("database").(string)
	table := d.Get("table").(string)
	partition := d.Get("partition").(string)
	placementPolicy := d.Get("placement_policy").(string)
	if err := setTiPartitionPlacementPolicy(ctx, db, database, table, partition, placementPolicy); err != nil {
		return diag.Errorf("error setting TiDB placement policy (%s) on partition (%s.%s.%s): %s", placementPolicy, database, table, partition, err)
	}

	d.SetId(formatTiPartitionPlacementPolicyID(database, table, partition))
	return ReadTiPartitionPlacementPolicy(ctx, d, meta)
}

func ReadTiPartitionPlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "partition placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database, table, partition, err := tiPartitionPlacementPolicyFields(d)
	if err != nil {
		return diag.FromErr(err)
	}

	placementPolicy, exists, err := readTiPartitionPlacementPolicyFromDB(ctx, db, database, table, partition)
	if err != nil {
		return diag.Errorf("error reading TiDB partition placement policy (%s.%s.%s): %s", database, table, partition, err)
	}
	if !exists {
		d.SetId("")
		return nil
	}

	d.Set("database", database)
	d.Set("table", table)
	d.Set("partition", partition)
	d.Set("placement_policy", placementPolicy)
	d.SetId(formatTiPartitionPlacementPolicyID(database, table, partition))
	return nil
}

func DeleteTiPartitionPlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	if err := requireTiDB(ctx, db, "partition placement policies"); err != nil {
		return diag.FromErr(err)
	}

	database, table, partition, err := tiPartitionPlacementPolicyFields(d)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := setTiPartitionPlacementPolicy(ctx, db, database, table, partition, placementPolicyDefault); err != nil {
		if mysqlErrorNumber(err) == unknownDatabaseErrCode || mysqlErrorNumber(err) == unknownTableErrCode {
			d.SetId("")
			return nil
		}
		return diag.Errorf("error resetting TiDB placement policy on partition (%s.%s.%s): %s", database, table, partition, err)
	}

	d.SetId("")
	return nil
}

func ImportTiDatabasePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	database, err := parseTiDatabasePlacementPolicyID(d.Id())
	if err != nil {
		return nil, err
	}

	d.Set("database", database)
	return []*schema.ResourceData{d}, nil
}

func ImportTiTablePlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	database, table, err := parseTiTablePlacementPolicyID(d.Id())
	if err != nil {
		return nil, err
	}

	d.Set("database", database)
	d.Set("table", table)
	return []*schema.ResourceData{d}, nil
}

func ImportTiPartitionPlacementPolicy(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	database, table, partition, err := parseTiPartitionPlacementPolicyID(d.Id())
	if err != nil {
		return nil, err
	}

	d.Set("database", database)
	d.Set("table", table)
	d.Set("partition", partition)
	return []*schema.ResourceData{d}, nil
}

func setTiDatabasePlacementPolicy(ctx context.Context, db *sql.DB, database string, placementPolicy string) error {
	query := fmt.Sprintf("ALTER DATABASE %s PLACEMENT POLICY=%s", quoteIdentifier(database), tiDBPlacementPolicyClause(placementPolicy))
	log.Printf("[DEBUG] SQL: %s", query)
	_, err := db.ExecContext(ctx, query)
	return err
}

func setTiTablePlacementPolicy(ctx context.Context, db *sql.DB, database string, table string, placementPolicy string) error {
	query := fmt.Sprintf("ALTER TABLE %s PLACEMENT POLICY=%s", quoteQualifiedTable(database, table), tiDBPlacementPolicyClause(placementPolicy))
	log.Printf("[DEBUG] SQL: %s", query)
	_, err := db.ExecContext(ctx, query)
	return err
}

func setTiPartitionPlacementPolicy(ctx context.Context, db *sql.DB, database string, table string, partition string, placementPolicy string) error {
	query := fmt.Sprintf("ALTER TABLE %s PARTITION %s PLACEMENT POLICY=%s", quoteQualifiedTable(database, table), quoteIdentifier(partition), tiDBPlacementPolicyClause(placementPolicy))
	log.Printf("[DEBUG] SQL: %s", query)
	_, err := db.ExecContext(ctx, query)
	return err
}

func readTiDatabasePlacementPolicyFromDB(ctx context.Context, db *sql.DB, database string) (string, bool, error) {
	row, err := querySingleRowStringMapContext(ctx, db, `
SELECT TIDB_PLACEMENT_POLICY_NAME
FROM information_schema.schemata
WHERE schema_name = ?`, database)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	} else if err == nil {
		return normalizeTiDBPlacementPolicyReadback(stringMapValue(row, "TIDB_PLACEMENT_POLICY_NAME")), true, nil
	} else if mysqlErrorNumber(err) != unknownColumnErrCode {
		return "", false, err
	}

	stmtSQL := "SHOW CREATE DATABASE " + quoteIdentifier(database)
	var createSQL, _database string
	err = db.QueryRowContext(ctx, stmtSQL).Scan(&_database, &createSQL)
	if err != nil {
		if mysqlErrorNumber(err) == unknownDatabaseErrCode {
			return "", false, nil
		}
		return "", false, err
	}

	placementPolicyMatches := placementPolicyRegex.FindStringSubmatch(createSQL)
	if len(placementPolicyMatches) >= 2 {
		return placementPolicyMatches[1], true, nil
	}

	return placementPolicyDefault, true, nil
}

func readTiTablePlacementPolicyFromDB(ctx context.Context, db *sql.DB, database string, table string) (string, bool, error) {
	row, err := querySingleRowStringMapContext(ctx, db, `
SELECT TIDB_PLACEMENT_POLICY_NAME
FROM information_schema.tables
WHERE table_schema = ? AND table_name = ?`, database, table)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}

	return normalizeTiDBPlacementPolicyReadback(stringMapValue(row, "TIDB_PLACEMENT_POLICY_NAME")), true, nil
}

func readTiPartitionPlacementPolicyFromDB(ctx context.Context, db *sql.DB, database string, table string, partition string) (string, bool, error) {
	row, err := querySingleRowStringMapContext(ctx, db, `
SELECT TIDB_PLACEMENT_POLICY_NAME
FROM information_schema.partitions
WHERE table_schema = ? AND table_name = ? AND partition_name = ?`, database, table, partition)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}

	return normalizeTiDBPlacementPolicyReadback(stringMapValue(row, "TIDB_PLACEMENT_POLICY_NAME")), true, nil
}

func normalizeTiDBPlacementPolicyReadback(placementPolicy string) string {
	if placementPolicy == "" {
		return placementPolicyDefault
	}

	return placementPolicy
}

func tiDBPlacementPolicyClause(placementPolicy string) string {
	if strings.EqualFold(placementPolicy, placementPolicyDefault) {
		return placementPolicyDefault
	}

	return quoteIdentifier(placementPolicy)
}

func quoteQualifiedTable(database string, table string) string {
	return quoteIdentifier(database) + "." + quoteIdentifier(table)
}

func tiDatabasePlacementPolicyFields(d *schema.ResourceData) (string, error) {
	if database := d.Get("database").(string); database != "" {
		return database, nil
	}

	return parseTiDatabasePlacementPolicyID(d.Id())
}

func tiTablePlacementPolicyFields(d *schema.ResourceData) (string, string, error) {
	database := d.Get("database").(string)
	table := d.Get("table").(string)
	if database != "" && table != "" {
		return database, table, nil
	}

	return parseTiTablePlacementPolicyID(d.Id())
}

func tiPartitionPlacementPolicyFields(d *schema.ResourceData) (string, string, string, error) {
	database := d.Get("database").(string)
	table := d.Get("table").(string)
	partition := d.Get("partition").(string)
	if database != "" && table != "" && partition != "" {
		return database, table, partition, nil
	}

	return parseTiPartitionPlacementPolicyID(d.Id())
}

func formatTiDatabasePlacementPolicyID(database string) string {
	return database
}

func formatTiTablePlacementPolicyID(database string, table string) string {
	return formatTiPlacementPolicyIDParts(database, table)
}

func formatTiPartitionPlacementPolicyID(database string, table string, partition string) string {
	return formatTiPlacementPolicyIDParts(database, table, partition)
}

func parseTiDatabasePlacementPolicyID(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("expected import ID in the format <database>")
	}

	return id, nil
}

func parseTiTablePlacementPolicyID(id string) (string, string, error) {
	parts, err := parseTiPlacementPolicyIDParts(id, 2, "<database>.<table>")
	if err != nil {
		return "", "", err
	}

	return parts[0], parts[1], nil
}

func parseTiPartitionPlacementPolicyID(id string) (string, string, string, error) {
	parts, err := parseTiPlacementPolicyIDParts(id, 3, "<database>.<table>.<partition>")
	if err != nil {
		return "", "", "", err
	}

	return parts[0], parts[1], parts[2], nil
}

func formatTiPlacementPolicyIDParts(parts ...string) string {
	escapedParts := make([]string, len(parts))
	for i, part := range parts {
		escapedParts[i] = escapeTiPlacementPolicyIDPart(part)
	}

	return strings.Join(escapedParts, ".")
}

func escapeTiPlacementPolicyIDPart(part string) string {
	return strings.NewReplacer(`\`, `\\`, `.`, `\.`).Replace(part)
}

func parseTiPlacementPolicyIDParts(id string, expectedParts int, format string) ([]string, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("expected import ID in the format %s", format)
	}

	parts := []string{}
	var current strings.Builder
	escaped := false
	for _, r := range id {
		if escaped {
			if r != '.' && r != '\\' {
				return nil, fmt.Errorf("invalid escape sequence \\%c in import ID %q; escape only literal dots as \\. and literal backslashes as \\\\", r, id)
			}
			current.WriteRune(r)
			escaped = false
			continue
		}

		switch r {
		case '\\':
			escaped = true
		case '.':
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if escaped {
		return nil, fmt.Errorf("invalid trailing escape in import ID %q", id)
	}

	parts = append(parts, current.String())
	if len(parts) != expectedParts {
		return nil, fmt.Errorf("expected import ID in the format %s", format)
	}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("expected import ID in the format %s", format)
		}
	}

	return parts, nil
}

func requireTiDB(ctx context.Context, db *sql.DB, feature string) error {
	isTiDB, _, _, err := serverTiDB(db)
	if err != nil {
		return err
	}
	if !isTiDB {
		return fmt.Errorf("%s are only supported for TiDB", feature)
	}

	return nil
}
