package main

import (
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/cmd/benchmark/commands"
	"github.com/mitchellh/cli"
)

func main() {

	rand.Seed(time.Now().Unix())
	cli := &cli.CLI{
		Name:     "libp2p-framework",
		Args:     os.Args[1:],
		Commands: commands.GetGossipCommands(),
	}

	_, err := cli.Run()
	if err != nil {
		log.Fatalf("Error executing CLI: %s\n", err.Error())
	}
}
