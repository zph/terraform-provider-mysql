package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/zph/terraform-provider-mysql/v3/internal/testmatrix"
)

func main() {
	githubActions := flag.Bool("github-actions", false, "emit GitHub Actions strategy matrix JSON")
	flag.Parse()

	if !*githubActions {
		fmt.Fprintln(os.Stderr, "ERROR: pass --github-actions")
		os.Exit(2)
	}

	payload, err := json.Marshal(testmatrix.ActionsMatrix())
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: marshal test matrix: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(payload))
}
