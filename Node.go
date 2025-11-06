package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	mutualexclusion "github.com/Gustav2620/Ricart-Argawala/proto"

	"google.golang.org/grpc"
)

func loadPeers(filename string, selfPort int) []string {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Cannot open peers file: %v", err)
	}
	defer file.Close()

	var peers []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		port, _ := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if port != selfPort {
			peers = append(peers, fmt.Sprintf("localhost:%d", port))
		}
	}
	return peers
}

func (n *Node) updateLamport(t int64) {
	n.mu.Lock()
	if t > n.lamportTime {
		n.lamportTime = t
	}
	n.lamportTime++
	n.mu.Unlock()
}

func (n *Node) RequestCriticalSection() {
	n.mu.Lock()
	n.state = Wanted
	n.lamportTime++
	T := n.lamportTime
	n.replyCount = 0
	req := &mutualexclusion.RequestMsg{
		NodeId:    int32(n.id),
		Timestamp: T,
		Port:      int32(n.port),
	}
	n.mu.Unlock()

	fmt.Printf("[Node %d] REQUESTING CS at Lamport time %d\n", n.id, T)

	// Multicast REQUEST to all peers
	for _, peer := range n.peers {
		go n.sendRequest(peer, req)
	}

	// Wait for N-1 replies
	n.waitForReplies(len(n.peers))
}

func (n *Node) sendRequest(peer string, req *mutualexclusion.RequestMsg) {
	conn, err := grpc.Dial(peer,
		grpc.WithInsecure(),
		grpc.WithTimeout(2*time.Second), // replaces WithBlockTimeout
	)
	if err != nil {
		return
	}
	defer conn.Close()

	client := mutualexclusion.NewMutualExclusionClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = client.RequestEntry(ctx, req)
	if err != nil {
		return
	}

	// count reply
	n.mu.Lock()
	n.replyCount++
	n.mu.Unlock()
}

func (n *Node) waitForReplies(expected int) {
	for {
		n.mu.Lock()
		if n.replyCount >= expected {
			n.state = Held
			n.mu.Unlock()
			n.enterCriticalSection()
			return
		}
		n.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
}

func (n *Node) enterCriticalSection() {
	fmt.Printf(">>> [Node %d] ENTERED CRITICAL SECTION <<<\n", n.id)
	time.Sleep(2 * time.Second) // Emulate CS work
	fmt.Printf("<<< [Node %d] EXITING CRITICAL SECTION >>>\n", n.id)

	n.mu.Lock()
	n.state = Released
	queue := n.queue
	n.queue = nil
	n.pendingReq = nil
	n.mu.Unlock()

	// Reply to all deferred requests
	for _, req := range queue {
		go n.sendReply(fmt.Sprintf("localhost:%d", req.Port))
	}
}

func (n *Node) sendReply(addr string) {
	conn, err := grpc.Dial(addr,
		grpc.WithInsecure(),
		grpc.WithTimeout(1*time.Second), // replaces WithBlockTimeout
	)
	if err != nil {
		return
	}
	defer conn.Close()

	client := mutualexclusion.NewMutualExclusionClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// we only need to trigger the receiver – any message works
	client.RequestEntry(ctx, &mutualexclusion.RequestMsg{})
}

// gRPC Handler
func (n *Node) RequestEntry(ctx context.Context, req *mutualexclusion.RequestMsg) (*mutualexclusion.ReplyMsg, error) {
	receivedTime := req.Timestamp
	n.updateLamport(receivedTime)

	n.mu.Lock()
	defer n.mu.Unlock()

	// On receive 'req(Ti,pi)'
	if n.state == Held || (n.state == Wanted && (n.lamportTime < receivedTime || (n.lamportTime == receivedTime && n.id < int(req.NodeId)))) {
		// Queue the request
		n.queue = append(n.queue, RequestMsg{
			NodeID:    int(req.NodeId),
			Timestamp: receivedTime,
			Port:      int(req.Port),
		})
		fmt.Printf("[Node %d] DEFERRED reply to Node %d (T=%d)\n", n.id, req.NodeId, receivedTime)
		return &mutualexclusion.ReplyMsg{Granted: false}, nil
	} else {
		// Immediate reply
		fmt.Printf("[Node %d] REPLYING to Node %d (T=%d)\n", n.id, req.NodeId, receivedTime)
		go n.sendReply(fmt.Sprintf("localhost:%d", req.Port))
		return &mutualexclusion.ReplyMsg{Granted: true}, nil
	}
}

func startServer(n *Node) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", n.port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	s := grpc.NewServer()
	mutualexclusion.RegisterMutualExclusionServer(s, n)
	fmt.Printf("[Node %d] Listening on port %d\n", n.id, n.port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
