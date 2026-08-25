package main

import (
	"os"

	"example.com/dynamis-code/apps-template/internal/appctl"
)

func main() {
	os.Exit(appctl.Run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
