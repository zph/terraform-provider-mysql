package main

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
	"github.com/petoju/terraform-provider-mysql/v3/mysql"
  // Added to ensure we have tfplugindocs available in go.mod
  _ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: mysql.Provider})
}
