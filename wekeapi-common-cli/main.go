package main

import (
	"os"

	"wekeapi-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
