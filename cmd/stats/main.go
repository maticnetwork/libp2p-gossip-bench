package main

import (
	"log"
	"os"

	"github.com/maticnetwork/libp2p-gossip-bench/cmd/stats/commands"
	"github.com/mitchellh/cli"
)

func main() {
	cli := &cli.CLI{
		Name:     "libp2p-framework",
		Args:     os.Args[1:],
		Commands: commands.GetStatsCommands(),
	}

	_, err := cli.Run()
	if err != nil {
		log.Fatalf("Error executing CLI: %s\n", err.Error())
	}
}
