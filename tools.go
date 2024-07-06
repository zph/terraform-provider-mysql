//go:build tools

/*
  Used for keeping ftplugindocs in the go.mod when using go mod tidy
  and used as go:generate
  Recommended approach per: https://github.com/golang/go/issues/25922#issuecomment-413898264
*/
package main

import (
    _ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
