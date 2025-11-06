package main

import (
	"flag"
	"sync"
	"time"

	mutualexclusion "github.com/Gustav2620/Ricart-Argawala/proto"
)

var (
	nodeID    = flag.Int("id", 1, "Node ID")
	port      = flag.Int("port", 50051, "Port to listen on")
	peersFile = flag.String("peers", "peers.txt", "File with peer ports")
)

type Node struct {
	mutualexclusion.UnimplementedMutualExclusionServer
	id          int
	port        int
	state       State
	lamportTime int64
	mu          sync.Mutex

	replyCount int
	pendingReq *RequestMsg
	queue      []RequestMsg

	peers []string // list of "localhost:port"
}

type State int

const (
	Released State = iota
	Wanted
	Held
)

type RequestMsg struct {
	NodeID    int
	Timestamp int64
	Port      int
}

func main() {
	flag.Parse()

	peers := loadPeers(*peersFile, *port)
	node := &Node{
		id:    *nodeID,
		port:  *port,
		state: Released,
		peers: peers,
	}

	go startServer(node)
	time.Sleep(1 * time.Second) // let server start

	// Simulate CS request after random delay
	time.Sleep(time.Duration(node.id) * 2 * time.Second)
	node.RequestCriticalSection()

	select {}
}
