package main

import (
	"flag"
	"fmt"
	"time"
)

func main() {
	id := flag.Int("id", 0, "Node ID (1, 2 or 3)")
	configFile := flag.String("config", "config.yaml", "Config file")
	flag.Parse()

	if *id < 1 || *id > 3 {
		fmt.Printf("Use -id 1, 2 or 3\n")
	}

	cfg, err := LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
	}

	myAddr, peers := BuildNodeMap(*id, cfg)
	node := NewNode(*id, myAddr, peers)

	go node.StartServer()
	time.Sleep(3 * time.Second) // Give servers time to start

	fmt.Printf("[Node %d] ONLINE | Listening on %s | Peers: %d\n", node.id, myAddr, len(peers))
	node.Run()
}