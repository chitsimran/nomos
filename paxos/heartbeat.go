package paxos

import (
	"log"
	"time"
)

type PingArgs struct {
	FromID int
}

type PingReply struct {
	OK        bool
	Timestamp time.Time
}

func (n *Node) HandlePing(args *PingArgs, reply *PingReply) error {
	log.Printf("Node %d received ping from node %d", n.ID, args.FromID)
	n.mu.Lock()
	n.lastHeartbeat[args.FromID] = time.Now()
	n.mu.Unlock()

	reply.OK = true
	reply.Timestamp = time.Now()
	return nil
}
