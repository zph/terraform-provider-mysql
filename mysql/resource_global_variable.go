package mysql

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceGlobalVariable() *schema.Resource {
	return &schema.Resource{
		Create: CreateGlobalVariable,
		Read:   ReadGlobalVariable,
		Update: UpdateGlobalVariable,
		Delete: DeleteGlobalVariable,
		Importer: &schema.ResourceImporter{
			State: ImportGlobalVariable,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"value": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func CreateGlobalVariable(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	name := d.Get("name").(string)
	value := d.Get("value").(string)

	sql := fmt.Sprintf("SET GLOBAL %s = %s", quoteIdentifier(name), quoteIdentifier(value))
	log.Printf("[DEBUG] SQL: %s", sql)

	_, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("error setting value: %s", err)
	}

	d.SetId(name)

	return nil
}

func ReadGlobalVariable(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	stmt, err := db.Prepare("SHOW GLOBAL VARIABLES WHERE VARIABLE_NAME = ?")
	if err != nil {
		log.Fatal(err)
	}

	var name, value string
	err = stmt.QueryRow(d.Id()).Scan(&name, &value)

	if err != nil && err != sql.ErrNoRows {
		d.SetId("")
		return fmt.Errorf("Error during show global variables: %s", err)
	}

	d.Set("name", name)
	d.Set("value", value)

	return nil
}

func UpdateGlobalVariable(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	name := d.Get("name").(string)
	value := d.Get("value").(string)

	sql := fmt.Sprintf("SET GLOBAL %s = %s", quoteIdentifier(name), quoteIdentifier(value))
	log.Printf("[DEBUG] SQL: %s", sql)

	_, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("error update value: %s", err)
	}
	return ReadGlobalVariable(d, meta)
}

func DeleteGlobalVariable(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db
	name := d.Get("name").(string)

	sql := fmt.Sprintf("SET GLOBAL %s = DEFAULT", quoteIdentifier(name))
	log.Printf("[DEBUG] SQL: %s", sql)

	_, err := db.Exec(sql)
	if err != nil {
		log.Printf("[WARN] Variable_name (%s) not found; removing from state", d.Id())
		d.SetId("")
		return nil
	}

	return nil
}

func ImportGlobalVariable(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	err := ReadGlobalVariable(d, meta)

	if err != nil {
		return nil, err
	}

	return []*schema.ResourceData{d}, nil
}
