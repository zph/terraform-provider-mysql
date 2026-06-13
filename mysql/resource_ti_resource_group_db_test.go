package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestGetResourceGroupFromDBParsesResourceUnits(t *testing.T) {
	tests := []struct {
		name    string
		raw     driver.Value
		want    int
		wantErr bool
	}{
		{
			name: "numeric",
			raw:  []byte("2000000"),
			want: 2000000,
		},
		{
			name: "unlimited",
			raw:  []byte("UNLIMITED"),
			want: tiDBUnlimitedResourceUnits,
		},
		{
			name:    "invalid",
			raw:     []byte("bad-value"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openResourceGroupTestDB(t, []driver.Value{"rg_test", tt.raw, "medium", true, ""})
			rg, err := getResourceGroupFromDB(db, "rg_test")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rg.ResourceUnits != tt.want {
				t.Fatalf("got %d, want %d", rg.ResourceUnits, tt.want)
			}
		})
	}
}

func TestReadResourceGroupRefreshesUnlimitedResourceUnitsFromExistingMaxIntState(t *testing.T) {
	resourceName := "rg_test"
	db := openResourceGroupTestDB(t, []driver.Value{resourceName, []byte("UNLIMITED"), "medium", true, ""})

	conf := &MySQLConfiguration{
		Config: &mysqlDriver.Config{
			User: "root",
			Net:  "tcp",
			Addr: "resource-group-refresh-test",
		},
	}
	dsn := conf.Config.FormatDSN()

	connectionCacheMtx.Lock()
	connectionCache[dsn] = &OneConnection{Db: db}
	connectionCacheMtx.Unlock()
	t.Cleanup(func() {
		connectionCacheMtx.Lock()
		delete(connectionCache, dsn)
		connectionCacheMtx.Unlock()
	})

	resource := resourceTiResourceGroup()
	data := schema.TestResourceDataRaw(t, resource.Schema, map[string]interface{}{
		"name":           resourceName,
		"resource_units": tiDBUnlimitedResourceUnits,
		"priority":       "medium",
		"burstable":      true,
		"query_limit":    "",
	})
	data.SetId(resourceName)

	diags := ReadResourceGroup(context.Background(), data, conf)
	if diags.HasError() {
		t.Fatalf("refresh failed: %v", diags)
	}
	if data.Id() != resourceName {
		t.Fatalf("resource ID changed during refresh: got %q, want %q", data.Id(), resourceName)
	}
	if got := data.Get("resource_units").(int); got != tiDBUnlimitedResourceUnits {
		t.Fatalf("resource_units changed during refresh: got %d, want %d", got, tiDBUnlimitedResourceUnits)
	}
	if got := data.Get("burstable").(bool); !got {
		t.Fatalf("burstable changed during refresh: got %t, want true", got)
	}
}

const resourceGroupTestDriverName = "resource_group_test"

var (
	registerResourceGroupTestDriver sync.Once
	resourceGroupTestRowsByName     sync.Map
)

func openResourceGroupTestDB(t *testing.T, row []driver.Value) *sql.DB {
	t.Helper()

	registerResourceGroupTestDriver.Do(func() {
		sql.Register(resourceGroupTestDriverName, resourceGroupTestDriver{})
	})

	resourceGroupTestRowsByName.Store(t.Name(), row)
	t.Cleanup(func() {
		resourceGroupTestRowsByName.Delete(t.Name())
	})

	db, err := sql.Open(resourceGroupTestDriverName, t.Name())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	return db
}

type resourceGroupTestDriver struct{}

func (d resourceGroupTestDriver) Open(name string) (driver.Conn, error) {
	row, ok := resourceGroupTestRowsByName.Load(name)
	if !ok {
		return nil, fmt.Errorf("no test row registered for %s", name)
	}

	return &resourceGroupTestConn{row: row.([]driver.Value)}, nil
}

type resourceGroupTestConn struct {
	row []driver.Value
}

func (c *resourceGroupTestConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *resourceGroupTestConn) Close() error {
	return nil
}

func (c *resourceGroupTestConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *resourceGroupTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 query argument, got %d", len(args))
	}

	return &resourceGroupTestRows{row: c.row}, nil
}

type resourceGroupTestRows struct {
	row  []driver.Value
	read bool
}

func (r *resourceGroupTestRows) Columns() []string {
	return []string{"NAME", "RU_PER_SEC", "PRIORITY", "BURSTABLE", "QUERY_LIMIT"}
}

func (r *resourceGroupTestRows) Close() error {
	return nil
}

func (r *resourceGroupTestRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}

	copy(dest, r.row)
	r.read = true
	return nil
}
