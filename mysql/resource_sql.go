package mysql

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceSql() *schema.Resource {
	return &schema.Resource{
		CreateContext: CreateSql,
		ReadContext:   ReadSql,
		DeleteContext: DeleteSql,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"create_sql": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"delete_sql": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func CreateSql(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	name := d.Get("name").(string)
	createSql := d.Get("create_sql").(string)

	log.Println("Executing SQL", createSql)

	_, err = db.ExecContext(ctx, createSql)
	if err != nil {
		return diag.Errorf("couldn't exec SQL: %v", err)
	}

	d.SetId(name)

	return nil
}

func ReadSql(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func DeleteSql(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	db, err := getDatabaseFromMeta(ctx, meta)
	if err != nil {
		return diag.FromErr(err)
	}
	deleteSql := d.Get("delete_sql").(string)

	log.Println("Executing SQL:", deleteSql)

	_, err = db.ExecContext(ctx, deleteSql)
	if err != nil {
		return diag.Errorf("failed to run delete SQL: %v", err)
	}

	d.SetId("")
	return nil
}
