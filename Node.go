package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	pb "github.com/Gustav2620/Ricart-Argawala/grpc"
)

type DeferredRequest struct {
	ID   int
	Addr string
}

type Node struct {
	pb.UnimplementedMutualExclusionServer

	id          int
	addr        string
	peers       map[int]string
	clock       int64
	mu          sync.Mutex
	cond        *sync.Cond
	requesting  bool
	inCS        bool
	myTS        int64
	repliesRecv int
	deferred    []DeferredRequest
}

func NewNode(id int, addr string, peers map[int]string) *Node {
	n := &Node{
		id:       id,
		addr:     addr,
		peers:    peers,
		clock:    0,
		deferred: make([]DeferredRequest, 0),
	}
	n.cond = sync.NewCond(&n.mu)
	return n
}

func (n *Node) tick() {
	n.mu.Lock()
	n.clock++
	n.mu.Unlock()
}


func (n *Node) getClock() int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.clock
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (n *Node) ReceiveRequest(ctx context.Context, req *pb.Request) (*pb.Empty, error) {
	fmt.Printf("[Node %d] Received request from %d (their TS=%d) (LamportTime: %d)\n", n.id, req.SenderId, req.Timestamp, n.getClock())

	priority := n.requesting && (n.myTS < req.Timestamp || (n.myTS == req.Timestamp && int32(n.id) < req.SenderId))

	if priority || n.inCS {
		n.mu.Lock()
		n.deferred = append(n.deferred, DeferredRequest{ID: int(req.SenderId), Addr: req.SenderAddr})
		n.mu.Unlock()
		fmt.Printf("[Node %d] Deferred reply to %d (I have higher priority) (LamportTime: %d)\n", n.id, req.SenderId, n.getClock())
	} else {
		go n.sendReply(req.SenderAddr, int(req.SenderId))
	}
	return &pb.Empty{}, nil
}

func (n *Node) ReceiveReply(ctx context.Context, rep *pb.Reply) (*pb.Empty, error) {
	n.mu.Lock()
	n.repliesRecv++
	n.mu.Unlock()
	fmt.Printf("[Node %d] Received reply from %d (%d/%d replies) (LamportTime: %d)\n", n.id, rep.SenderId, n.repliesRecv, len(n.peers), n.getClock())
	n.cond.Signal()
	return &pb.Empty{}, nil
}

func (n *Node) sendReply(addr string, id int) {
	n.tick()
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		fmt.Printf("[Node %d] Dial failed to %d: %v (LamportTime: %d)\n", n.id, id, err, n.getClock())
		return
	}
	defer conn.Close()

	client := pb.NewMutualExclusionClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = client.ReceiveReply(ctx, &pb.Reply{SenderId: int32(n.id)})
	if err != nil {
		fmt.Printf("[Node %d] Failed to send reply to %d: %v (LamportTime: %d)\n", n.id, id, err, n.getClock())
	} else {
		fmt.Printf("[Node %d] Sent reply to %d (LamportTime: %d)\n", n.id, id, n.getClock())
	}
}

func (n *Node) RequestCS() {
	n.tick()
	n.mu.Lock()
	n.requesting = true
	n.myTS = n.clock
	n.repliesRecv = 0
	n.mu.Unlock()

	fmt.Printf("[Node %d] Requesting critical section (my TS=%d) (LamportTime: %d)\n", n.id, n.myTS, n.getClock())

	for peerID, peerAddr := range n.peers {
		go func(id int, addr string) {
			n.tick()
			conn, err := grpc.Dial(addr, grpc.WithInsecure())
			if err != nil {
				fmt.Printf("[Node %d] Failed connect to %d: %v (LamportTime: %d)\n", n.id, id, err, n.getClock())
				return
			}
			defer conn.Close()

			client := pb.NewMutualExclusionClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			_, err = client.ReceiveRequest(ctx, &pb.Request{
				Timestamp:  n.myTS,
				SenderId:   int32(n.id),
				SenderAddr: n.addr,
			})
			if err != nil {
				fmt.Printf("[Node %d] Failed send request to %d: %v (LamportTime: %d)\n", n.id, id, err, n.getClock())
			} else {
				fmt.Printf("[Node %d] Sent request to %d (LamportTime: %d)\n", n.id, id, n.getClock())
			}
		}(peerID, peerAddr)
	}

	n.mu.Lock()
	for n.repliesRecv < len(n.peers) {
		n.cond.Wait()
	}
	n.mu.Unlock()

	n.tick()
	n.mu.Lock()
	n.requesting = false
	n.inCS = true
	n.mu.Unlock()

	fmt.Printf("[Node %d] ENTERED CRITICAL SECTION (TS=%d) (LamportTime: %d)\n", n.id, n.myTS, n.getClock())
	fmt.Printf("NODE %d IN CRITICAL SECTION\n", n.id)

	time.Sleep(2 * time.Second)

	n.tick()
	fmt.Printf("[Node %d] EXITING CRITICAL SECTION (LamportTime: %d)\n", n.id, n.getClock())

	n.mu.Lock()
	for _, d := range n.deferred {
		go n.sendReply(d.Addr, d.ID)
	}
	n.deferred = nil
	n.inCS = false
	n.mu.Unlock()
}

func (n *Node) StartServer() {
	lis, err := net.Listen("tcp", n.addr)
	if err != nil {
		fmt.Printf("[Node %d] Failed to listen on %s: %v (LamportTime: %d)\n", n.id, n.addr, err, n.getClock())
		return
	}
	s := grpc.NewServer()
	pb.RegisterMutualExclusionServer(s, n)
	fmt.Printf("[Node %d] gRPC server started on %s (LamportTime: %d)\n", n.id, n.addr, n.getClock())
	if err := s.Serve(lis); err != nil {
		fmt.Printf("[Node %d] Server crashed: %v (LamportTime: %d)\n", n.id, err, n.getClock())
	}
}

func (n *Node) Run() {
	rand.Seed(time.Now().UnixNano() + int64(n.id)*100)
	for {
		sleep := time.Duration(rand.Intn(10)+5) * time.Second
		time.Sleep(sleep)
		n.RequestCS()
	}
}