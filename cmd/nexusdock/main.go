package main

import (
	"os"

	"github.com/uvwt/nexusdock/internal/nexusapp"
)

func main() {
	os.Exit(nexusapp.Main(os.Args))
}
