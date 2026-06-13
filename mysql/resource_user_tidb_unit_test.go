package mysql

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestBuildTiDBUserOptionClauses(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceUser().Schema, map[string]interface{}{
		"resource_group":          "rg_app",
		"account_locked":          true,
		"comment":                 "app user",
		"attribute_json":          `{"team":"platform"}`,
		"password_expire":         "interval 90 day",
		"password_history":        "5",
		"password_reuse_interval": "30 day",
		"failed_login_attempts":   3,
		"password_lock_time":      "unbounded",
	})

	got := strings.Join(buildTiDBUserOptionClauses(d, false), " ")
	want := `PASSWORD EXPIRE INTERVAL 90 DAY PASSWORD HISTORY 5 PASSWORD REUSE INTERVAL 30 DAY FAILED_LOGIN_ATTEMPTS 3 PASSWORD_LOCK_TIME UNBOUNDED ACCOUNT LOCK COMMENT 'app user' ATTRIBUTE '{"team":"platform"}' RESOURCE GROUP ` + "`rg_app`"
	if got != want {
		t.Fatalf("buildTiDBUserOptionClauses() = %q, want %q", got, want)
	}
}

func TestBuildTiDBUserOptionClausesOmittedBoolDoesNotEmitAccountUnlock(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceUser().Schema, map[string]interface{}{
		"resource_group": "rg_app",
	})

	got := strings.Join(buildTiDBUserOptionClauses(d, false), " ")
	want := "RESOURCE GROUP `rg_app`"
	if got != want {
		t.Fatalf("buildTiDBUserOptionClauses() = %q, want %q", got, want)
	}
}

func TestSetTiDBUserOptionsFromCreateStatement(t *testing.T) {
	d := schema.TestResourceDataRaw(t, resourceUser().Schema, map[string]interface{}{})
	stmt := "CREATE USER `jdoe`@`%` IDENTIFIED WITH `mysql_native_password` REQUIRE NONE PASSWORD EXPIRE INTERVAL 90 DAY PASSWORD HISTORY 5 PASSWORD REUSE INTERVAL 30 DAY FAILED_LOGIN_ATTEMPTS 3 PASSWORD_LOCK_TIME UNBOUNDED ACCOUNT LOCK COMMENT 'app user' ATTRIBUTE '{\"team\":\"platform\"}' RESOURCE GROUP `rg_app`"

	setTiDBUserOptionsFromCreateStatement(stmt, d)

	checks := map[string]interface{}{
		"resource_group":          "rg_app",
		"max_user_connections":    0,
		"account_locked":          true,
		"comment":                 "app user",
		"attribute_json":          `{"team":"platform"}`,
		"password_expire":         "interval 90 day",
		"password_history":        "5",
		"password_reuse_interval": "30 day",
		"failed_login_attempts":   3,
		"password_lock_time":      "unbounded",
	}

	for key, want := range checks {
		if got := d.Get(key); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
}
