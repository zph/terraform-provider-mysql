package mysql

import (
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestNormalizePerms(t *testing.T) {
	got := normalizePerms([]string{
		"`select (b, a)`",
		"USAGE",
		"allprivileges",
		"INSERT(c3, c1)",
	})
	want := []string{
		"ALL PRIVILEGES",
		"INSERT(C1, C3)",
		"SELECT(A, B)",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizePerms() = %#v, want %#v", got, want)
	}
}

func TestParseGrantFromRowTableGrant(t *testing.T) {
	grant, err := parseGrantFromRow("GRANT SELECT, INSERT(c2, c1) ON `app_db`.`accounts` TO 'app'@'%' REQUIRE SSL WITH GRANT OPTION")
	if err != nil {
		t.Fatalf("parseGrantFromRow returned error: %s", err)
	}

	tableGrant, ok := grant.(*TablePrivilegeGrant)
	if !ok {
		t.Fatalf("parseGrantFromRow returned %T, want *TablePrivilegeGrant", grant)
	}

	if tableGrant.Database != "app_db" {
		t.Fatalf("Database = %q, want %q", tableGrant.Database, "app_db")
	}
	if tableGrant.Table != "accounts" {
		t.Fatalf("Table = %q, want %q", tableGrant.Table, "accounts")
	}
	if !reflect.DeepEqual(tableGrant.Privileges, []string{"INSERT(C1, C2)", "SELECT"}) {
		t.Fatalf("Privileges = %#v", tableGrant.Privileges)
	}
	if !tableGrant.Grant {
		t.Fatal("Grant = false, want true")
	}
	if tableGrant.TLSOption != "SSL" {
		t.Fatalf("TLSOption = %q", tableGrant.TLSOption)
	}
	if !tableGrant.UserOrRole.Equals(UserOrRole{Name: "app", Host: "%"}) {
		t.Fatalf("UserOrRole = %#v", tableGrant.UserOrRole)
	}
}

func TestParseGrantFromRowProcedureGrant(t *testing.T) {
	grant, err := parseGrantFromRow("GRANT EXECUTE ON PROCEDURE `app_db`.`rotate_keys` TO 'app'@'localhost'")
	if err != nil {
		t.Fatalf("parseGrantFromRow returned error: %s", err)
	}

	procedureGrant, ok := grant.(*ProcedurePrivilegeGrant)
	if !ok {
		t.Fatalf("parseGrantFromRow returned %T, want *ProcedurePrivilegeGrant", grant)
	}

	if procedureGrant.ObjectT != kProcedure {
		t.Fatalf("ObjectT = %q, want %q", procedureGrant.ObjectT, kProcedure)
	}
	if procedureGrant.Database != "app_db" {
		t.Fatalf("Database = %q, want %q", procedureGrant.Database, "app_db")
	}
	if procedureGrant.CallableName != "rotate_keys" {
		t.Fatalf("CallableName = %q, want %q", procedureGrant.CallableName, "rotate_keys")
	}
	if !reflect.DeepEqual(procedureGrant.Privileges, []string{"EXECUTE"}) {
		t.Fatalf("Privileges = %#v", procedureGrant.Privileges)
	}
}

func TestParseGrantFromRowRoleGrant(t *testing.T) {
	grant, err := parseGrantFromRow("GRANT `writer`, `reader` TO 'app'@'localhost' WITH ADMIN OPTION")
	if err != nil {
		t.Fatalf("parseGrantFromRow returned error: %s", err)
	}

	roleGrant, ok := grant.(*RoleGrant)
	if !ok {
		t.Fatalf("parseGrantFromRow returned %T, want *RoleGrant", grant)
	}

	if !reflect.DeepEqual(roleGrant.Roles, []string{"writer", "reader"}) {
		t.Fatalf("Roles = %#v", roleGrant.Roles)
	}
	if !roleGrant.Grant {
		t.Fatal("Grant = false, want true")
	}
	if !roleGrant.UserOrRole.Equals(UserOrRole{Name: "app", Host: "localhost"}) {
		t.Fatalf("UserOrRole = %#v", roleGrant.UserOrRole)
	}
}

func TestParseGrantFromRowSkipsPartialRevoke(t *testing.T) {
	grant, err := parseGrantFromRow("REVOKE INSERT ON `app_db`.`accounts` FROM 'app'@'%'")
	if err != nil {
		t.Fatalf("parseGrantFromRow returned error: %s", err)
	}
	if grant != nil {
		t.Fatalf("grant = %#v, want nil", grant)
	}
}

func TestTablePrivilegeGrantSQLStatements(t *testing.T) {
	grant := &TablePrivilegeGrant{
		Database:   "app_db",
		Table:      "accounts",
		Privileges: []string{"SELECT", "UPDATE(c1, c2)"},
		Grant:      true,
		UserOrRole: UserOrRole{Name: "app", Host: "%"},
		TLSOption:  "SSL",
	}

	wantGrant := "GRANT SELECT, UPDATE(c1, c2) ON `app_db`.`accounts` TO 'app'@'%' REQUIRE SSL WITH GRANT OPTION"
	if got := grant.SQLGrantStatement(); got != wantGrant {
		t.Fatalf("SQLGrantStatement() = %q, want %q", got, wantGrant)
	}

	wantRevoke := "REVOKE SELECT, UPDATE(c1, c2), GRANT OPTION ON `app_db`.`accounts` FROM 'app'@'%'"
	if got := grant.SQLRevokeStatement(); got != wantRevoke {
		t.Fatalf("SQLRevokeStatement() = %q, want %q", got, wantRevoke)
	}
}

func TestParseResourceFromDataTableGrant(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceGrant().Schema, map[string]interface{}{
		"user":       "app",
		"host":       "%",
		"database":   "app_db",
		"table":      "accounts",
		"privileges": []interface{}{"update(c2, c1)", "select"},
		"grant":      true,
		"tls_option": "SSL",
	})

	grant, diagErr := parseResourceFromData(d)
	if diagErr.HasError() {
		t.Fatalf("parseResourceFromData returned diagnostics: %s", diagErr[0].Summary)
	}

	tableGrant, ok := grant.(*TablePrivilegeGrant)
	if !ok {
		t.Fatalf("parseResourceFromData returned %T, want *TablePrivilegeGrant", grant)
	}
	if !reflect.DeepEqual(tableGrant.Privileges, []string{"SELECT", "UPDATE(C1, C2)"}) {
		t.Fatalf("Privileges = %#v", tableGrant.Privileges)
	}
	if got := tableGrant.SQLGrantStatement(); got != "GRANT SELECT, UPDATE(C1, C2) ON `app_db`.`accounts` TO 'app'@'%' REQUIRE SSL WITH GRANT OPTION" {
		t.Fatalf("SQLGrantStatement() = %q", got)
	}
}

func TestParseResourceFromDataRoleGrant(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceGrant().Schema, map[string]interface{}{
		"role":       "app_role",
		"database":   "*",
		"roles":      []interface{}{"writer", "reader"},
		"grant":      true,
		"tls_option": "NONE",
	})

	grant, diagErr := parseResourceFromData(d)
	if diagErr.HasError() {
		t.Fatalf("parseResourceFromData returned diagnostics: %s", diagErr[0].Summary)
	}

	roleGrant, ok := grant.(*RoleGrant)
	if !ok {
		t.Fatalf("parseResourceFromData returned %T, want *RoleGrant", grant)
	}
	roles := append([]string(nil), roleGrant.Roles...)
	sort.Strings(roles)
	if !reflect.DeepEqual(roles, []string{"reader", "writer"}) {
		t.Fatalf("Roles = %#v", roleGrant.Roles)
	}
}
