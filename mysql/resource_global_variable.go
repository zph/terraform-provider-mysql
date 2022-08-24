package mysql

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceGlobalVariable() *schema.Resource {
	return &schema.Resource{
		Create: CreateOrUpdateGlobalVariable,
		Read:   ReadGlobalVariable,
		Update: CreateOrUpdateGlobalVariable,
		Delete: DeleteGlobalVariable,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
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
				ValidateFunc: func(val any, key string) (warns []string, errs []error) {
					value := val.(string)
					match, _ := regexp.MatchString("(^`(.*)`$|')", value)
					if match {
						errs = append(errs, fmt.Errorf("%q is badly formatted. %q can't contain any ' string or `<value>`, got: %s", key, key, value))
					}
					return
				},
			},
		},
	}
}

func CreateOrUpdateGlobalVariable(d *schema.ResourceData, meta interface{}) error {
	var sql string

	db := meta.(*MySQLConfiguration).Db
	name := d.Get("name").(string)
	value := d.Get("value").(string)

	sqlBaseQuery := fmt.Sprintf("SET GLOBAL %s = ", quoteIdentifier(name))

	// Detect number or string
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		sql = fmt.Sprintf("%s%s", sqlBaseQuery, value)
	} else {
		sql = fmt.Sprintf("%s'%s'", sqlBaseQuery, value)
	}

	log.Printf("[DEBUG] SQL: %s", sql)

	_, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("error setting value: %s", err)
	}

	d.SetId(name)

	return ReadGlobalVariable(d, meta)
}

func ReadGlobalVariable(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	stmt, err := db.Prepare("SHOW GLOBAL VARIABLES WHERE VARIABLE_NAME = ?")
	if err != nil {
		return fmt.Errorf("error during prepare statement for global variable: %s", err)
	}

	var name, value string
	err = stmt.QueryRow(d.Id()).Scan(&name, &value)

	if err != nil && err != sql.ErrNoRows {
		d.SetId("")
		return fmt.Errorf("error during show global variables: %s", err)
	}

	d.Set("name", name)
	d.Set("value", value)

	return nil
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
