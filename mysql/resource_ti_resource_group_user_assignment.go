package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceTiResourceGroupUserAssignment() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateOrUpdateResourceGroupUser,
		ReadContext:   ReadResourceGroupUser,
		UpdateContext: CreateOrUpdateResourceGroupUser,
		DeleteContext: DeleteResourceGroupUser,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"user": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"resource_group": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func CreateOrUpdateResourceGroupUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	// TODO: should this be the d.Id()?
	user := d.Get("user").(string)
	resourceGroup := d.Get("resource_group").(string)

	var warnLevel, warnMessage string
	var warnCode int = 0

	currentUser, _, err = readUserFromDB(db, user)
	if err != nil {
		d.SetId("")
		return diag.Errorf(`error during get user (%s): %s`, user, err)
	}

	if currentUser == "" {
		d.SetId("")
		return diag.Errorf(`must create user first before assigning to resource group | getting user %s | error %s`, currentUser, err)
	}

	sql := fmt.Sprintf("ALTER USER `%s` RESOURCE GROUP `%s`", user, resourceGroup)
	log.Printf("[DEBUG] SQL: %s\n", sql)

	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		d.SetId("")
		return diag.Errorf("error attaching user (%s) to resource group (%s): %s", user, resourceGroup, err)
	}

	// TODO: relevant?
	db.QueryRowContext(ctx, "SHOW WARNINGS").Scan(&warnLevel, &warnCode, &warnMessage)
	if warnCode != 0 {
		d.SetId("")
		return diag.Errorf("error setting value: %s -> %s Error: %s", user, resourceGroup, warnMessage)
	}

	d.SetId(user)
	return nil
}

func ReadResourceGroupUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var user, resourceGroup string

	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	user, resourceGroup, err = readUserFromDB(db, d.Id())
	if err != nil {
		d.SetId("")
		return diag.Errorf(`error getting user %s`, err)
	}

	// If the user doesn't exist, instead of erroring, recognize that there's
	// terraform drift and attempt to create the assignment again.
	if user == "" {
		d.SetId("")
		return nil
	}

	d.Set("user", user)
	d.Set("resource_group", resourceGroup)

	return nil
}

func DeleteResourceGroupUser(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	user := d.Get("user").(string)
	// TODO: should we re-assert that it's part of the expected resourceGroup first? I think no bc plan should read it

	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	deleteQuery := fmt.Sprintf("ALTER USER `%s` RESOURCE GROUP `default`", user)
	_, err = db.Exec(deleteQuery)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return diag.Errorf("error during drop resource group (%s): %s", d.Id(), err)
	}

	d.SetId("")
	return nil
}

func readUserFromDB(db *sql.DB, name string) (string, string, error) {
	selectUsersQuery := `SELECT USER, IFNULL(JSON_EXTRACT(User_attributes, "$.resource_group"), "") as resource_group FROM mysql.user WHERE USER = ?`
	row := db.QueryRow(selectUsersQuery, name)

	var user, resourceGroup string

	err := row.Scan(&user, &resourceGroup)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("[DEBUG] resource group doesn't exist (%s): %s", name, err)
		return "", "", nil
	} else if err != nil {
		return "", "", fmt.Errorf(`error fetching user %e`, err)
	}

	return user, resourceGroup, nil
}
