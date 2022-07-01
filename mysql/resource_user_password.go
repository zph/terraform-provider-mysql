package mysql

import (
	"fmt"
	"log"

	"github.com/gofrs/uuid"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func resourceUserPassword() *schema.Resource {
	return &schema.Resource{
		Create: SetUserPassword,
		Update: SetUserPassword,
		Read:   ReadUserPassword,
		Delete: DeleteUserPassword,
		Schema: map[string]*schema.Schema{
			"user": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"host": {
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: true,
				Default:  "localhost",
			},
			"plaintext_password": {
				Type:     schema.TypeString,
				Optional: true,
			},
		},
	}
}

func SetUserPassword(d *schema.ResourceData, meta interface{}) error {
	db := meta.(*MySQLConfiguration).Db

	uuid, err := uuid.NewV4()
	if err != nil {
		return err
	}

	password, passOk := d.GetOk("plaintext_password")
	if !passOk {
		password = uuid.String()
		d.Set("plaintext_password", password)
	}

	stmtSQL, err := getSetPasswordStatement(meta)
	if err != nil {
		return err
	}
	_, err = db.Exec(stmtSQL,
		d.Get("user").(string),
		d.Get("host").(string),
		password)
	if err != nil {
		return err
	}
	user := fmt.Sprintf("%s@%s",
		d.Get("user").(string),
		d.Get("host").(string))
	d.SetId(user)
	return nil
}

func canReadPassword(meta interface{}) (bool, error) {
	serverVersion := meta.(*MySQLConfiguration).Version
	ver, _ := version.NewVersion("8.0.0")
	return serverVersion.LessThan(ver), nil
}

func ReadUserPassword(d *schema.ResourceData, meta interface{}) error {
	canRead, err := canReadPassword(meta)
	if err != nil {
		return err
	}
	if !canRead {
		return nil
	}

	db := meta.(*MySQLConfiguration).Db

	results, err := db.Query(`SELECT IF(PASSWORD(?) = authentication_string,'OK','FAIL') result, plugin FROM mysql.user WHERE user = ? AND host = ?`,
		d.Get("plaintext_password").(string),
		d.Get("user").(string),
		d.Get("host").(string),
	)
	if err != nil {
		// For now, we expect we are root.
		return err
	}

	for results.Next() {
		var plugin string
		var correct string
		err = results.Scan(&plugin, &correct)
		if err != nil {
			return err
		}

		if plugin != "mysql_native_password" {
			// We don't know whether the password is fine; it probably is.
			return nil
		}

		if correct == "FAIL" {
			d.SetId("")
			return nil
		}

		if correct == "OK" {
			return nil
		}

		return fmt.Errorf("Unexpected result of query: correct: %v; plugin: %v", correct, plugin)
	}

	// User doesn't exist. Password is certainly wrong in mysql, destroy the resource.
	log.Printf("User and host doesn't exist %s@%s", d.Get("user").(string), d.Get("host").(string))
	d.SetId("")
	return nil
}

func DeleteUserPassword(d *schema.ResourceData, meta interface{}) error {
	// We don't need to do anything on the MySQL side here. Just need TF
	// to remove from the state file.
	return nil
}
