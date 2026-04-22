package main

import (
	"os"

	"github.com/shansongtech/iss-open-cli/cmd"
)

func main() {
	if cmd.Launch() {
		os.Exit(1)
	}
}
