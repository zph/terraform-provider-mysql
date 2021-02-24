package mysql

import (
	"fmt"

	"github.com/gofrs/uuid"
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

	stmtSQL, err := getSetPasswordStatement(db)
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

func ReadUserPassword(d *schema.ResourceData, meta interface{}) error {
	// This is obviously not possible.
	return nil
}

func DeleteUserPassword(d *schema.ResourceData, meta interface{}) error {
	// We don't need to do anything on the MySQL side here. Just need TF
	// to remove from the state file.
	return nil
}
