// Command update-go-output-manifest records approved source inputs after
// review. Do not run it merely to clear a conformance failure.
package main

import (
	"fmt"
	"os"

	"example.com/dynamis-code/apps-template/internal/platform/sourceaudit"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--verify" {
		for _, verify := range []func(string) error{sourceaudit.Verify, sourceaudit.VerifyBuildEnvironment} {
			if err := verify("."); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--unreviewed-ci" {
		fmt.Fprintln(os.Stderr, "unreviewed CI snapshot: not valid release review evidence")
		if err := sourceaudit.WriteReviewed("."); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 2 || os.Args[1] != "--reviewed" {
		fmt.Fprintln(os.Stderr, "review all Go output changes before running: go run ./scripts/update-go-output-manifest --reviewed")
		os.Exit(2)
	}
	if err := sourceaudit.WriteReviewed("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
