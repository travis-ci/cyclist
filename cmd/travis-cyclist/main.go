package main

import (
	"os"

	"github.com/travis-ci/cyclist"
)

func main() {
	cyclist.NewCLI().Run(os.Args)
}
