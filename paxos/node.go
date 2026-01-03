package paxos

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"nomos/kv"
	"sync"
	"time"
)

type Node struct {
	mu sync.Mutex

	ID    int
	Peers []string

	kv  *kv.Store
	wal *WAL

	currentBallot  Ballot
	promisedBallot Ballot

	isLeader     bool
	leaderBallot Ballot
	recovering   bool

	log map[int]AcceptedLogEntry

	commitIndex int
	lastApplied int
	nextSlot    int

	applyCond *sync.Cond

	heartbeatInterval time.Duration
	electionTimeout   time.Duration
	lastHeartbeat     map[int]time.Time
}

func NewNode(id int, peers []string) *Node {
	n := &Node{
		ID:    id,
		Peers: peers,

		kv: kv.NewStore(),

		currentBallot:  Ballot{Number: 0, NodeID: 0},
		promisedBallot: Ballot{Number: 0, NodeID: 0},

		isLeader:     false,
		leaderBallot: Ballot{Number: 0, NodeID: 0},
		recovering:   false,

		heartbeatInterval: time.Millisecond * 100,
		electionTimeout:   time.Millisecond * 500,

		log:           make(map[int]AcceptedLogEntry),
		lastHeartbeat: make(map[int]time.Time),

		commitIndex: 0,
		lastApplied: 0,
		nextSlot:    1,
	}
	n.applyCond = sync.NewCond(&n.mu)
	return n
}

func (n *Node) SetWAL(wal *WAL) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.wal = wal
}

func (n *Node) LoadFromWAL() error {
	if n.wal == nil {
		return nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.log = make(map[int]AcceptedLogEntry)
	n.commitIndex = 0
	n.lastApplied = 0
	n.isLeader = false
	n.recovering = false

	err := n.wal.Replay(func(rec WalRecord) error {
		switch rec.Type {

		case WalPromise:
			if rec.PromisedBallot.GreaterThan(n.promisedBallot) {
				n.promisedBallot = rec.PromisedBallot
			}

		case WalAccept:
			existing, ok := n.log[rec.Slot]
			if !ok || rec.Ballot.GreaterThan(existing.Ballot) {
				n.log[rec.Slot] = AcceptedLogEntry{
					Ballot: rec.Ballot,
					Value:  rec.Value,
				}
			}

		case WalCommit:
			if rec.CommitIndex > n.commitIndex {
				n.commitIndex = rec.CommitIndex
			}
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error loading WAL record:", err)
		return err
	}

	maxSlot := 0
	for slot := range n.log {
		if slot > maxSlot {
			maxSlot = slot
		}
	}
	n.nextSlot = maxSlot + 1
	n.applyCond.Signal()
	return nil
}

func (n *Node) RunPrepare() {
	n.mu.Lock()
	n.currentBallot.Increment(n.ID)
	ballot := n.currentBallot
	n.mu.Unlock()

	log.Printf("Starting Leader Election with Ballot %d", ballot)

	prepareArgs := PrepareArgs{Ballot: ballot}
	promised := 0

	recoveredLog := make(map[int]AcceptedLogEntry)
	nextSlot := 0

	for _, peer := range n.Peers {
		reply, ok := CallPrepare(peer, &prepareArgs)

		if !ok {
			continue
		}

		if reply.Promised {
			promised++
			for i, v := range reply.Accepted {
				if _, ok := recoveredLog[i]; !ok {
					recoveredLog[i] = v
				} else if v.Ballot.GreaterThan(recoveredLog[i].Ballot) {
					recoveredLog[i] = v
				}
				nextSlot = max(nextSlot, i)
			}
		} else if reply.PromisedBallot.GreaterThan(n.currentBallot) {
			n.mu.Lock()
			log.Printf("I'm NO LONGER the leader :|")
			n.isLeader = false
			n.mu.Unlock()
			return
		}
	}
	promised++
	N := len(n.Peers) + 1
	if promised >= N/2+1 {
		n.mu.Lock()
		log.Printf("I'm the leader now!")
		n.isLeader = true
		n.leaderBallot = ballot
		n.promisedBallot = ballot
		n.recovering = true
		for slot, entry := range recoveredLog {
			if _, ok := n.log[slot]; !ok {
				err := n.wal.Append(WalRecord{
					Type:   WalAccept,
					Slot:   slot,
					Ballot: entry.Ballot,
					Value:  entry.Value,
				})
				if err != nil {
					log.Printf("SAFETY VIOLATION! Error appending entry: %v", err)
				}

				n.log[slot] = entry
			}
		}
		for slot := range n.log {
			nextSlot = max(nextSlot, slot)
		}
		n.nextSlot = nextSlot + 1
		n.mu.Unlock()
		for i := n.commitIndex + 1; i < n.nextSlot; i++ {
			entry, ok := n.log[i]
			if !ok {
				continue
			}
			args := &AcceptArgs{Ballot: ballot, Slot: i, Value: entry.Value}
			err := n.runAcceptForSlot(args)
			if err != nil {
				log.Printf("Error accepting log for %d: %v", i, err)
			}
		}
		n.mu.Lock()
		n.recovering = false
		n.applyCond.Signal()
		n.mu.Unlock()
		go n.heartbeatLoop()
	}
}

func (n *Node) HandlePrepare(args *PrepareArgs, reply *PrepareReply) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.promisedBallot.GreaterThan(args.Ballot) {
		reply.Promised = false
		reply.PromisedBallot = n.promisedBallot
		return nil
	}

	if args.Ballot.NodeID == 0 {
		fmt.Printf("Got Ballot with Node ID 0\n")
	}

	err := n.wal.Append(WalRecord{Type: WalPromise, PromisedBallot: args.Ballot})
	if err != nil {
		reply.Promised = false
		reply.PromisedBallot = n.promisedBallot
		return err
	}
	n.promisedBallot = args.Ballot
	n.leaderBallot = args.Ballot
	reply.Promised = true
	reply.PromisedBallot = n.promisedBallot
	reply.Accepted = make(map[int]AcceptedLogEntry, len(n.log))
	for slot, entry := range n.log {
		reply.Accepted[slot] = entry
	}
	return nil
}

func (n *Node) runAcceptForSlot(args *AcceptArgs) error {
	accepted := 0

	for _, peer := range n.Peers {
		reply, ok := CallAccept(peer, args)
		if !ok {
			continue
		}

		if !reply.Accepted {
			if reply.PromisedBallot.GreaterThan(args.Ballot) {
				n.mu.Lock()
				n.isLeader = false
				n.mu.Unlock()
				return nil
			}
			continue
		} else {
			accepted++
		}
	}
	selfReply := &AcceptReply{}

	n.HandleAccept(args, selfReply)
	if selfReply.Accepted {
		accepted++
	}

	N := len(n.Peers) + 1
	if accepted >= N/2+1 {
		log.Printf("Got majority OKs for Accept for slot %d", args.Slot)
		n.mu.Lock()
		commitArgs := CommitArgs{
			LeaderBallot: n.leaderBallot,
			CommitIndex:  args.Slot,
		}
		n.mu.Unlock()
		for _, peer := range n.Peers {
			_, ok := CallCommit(peer, &commitArgs)
			if !ok {
			}
		}
		selfCommitReply := &CommitReply{}

		n.HandleCommit(&commitArgs, selfCommitReply)
	} else {
		log.Printf("Didn't get majority OKs for Accept")
	}
	return nil
}

func (n *Node) RunAccept(command Command) error {
	n.mu.Lock()

	if !n.isLeader {
		// redirect
		n.mu.Unlock()
		log.Printf("I'm not the leader, send command at leader instead leaderID=%d", n.promisedBallot.NodeID)
		return nil
	}

	if n.recovering {
		n.mu.Unlock()
		return errors.New("leader recovering, retry")
	}

	slot := n.nextSlot
	n.nextSlot++

	args := &AcceptArgs{Ballot: n.leaderBallot, Slot: slot, Value: command}

	n.mu.Unlock()

	return n.runAcceptForSlot(args)
}

func (n *Node) HandleAccept(args *AcceptArgs, reply *AcceptReply) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.Ballot.LessThan(n.promisedBallot) {
		reply.Accepted = false
		reply.PromisedBallot = n.promisedBallot
		log.Printf("Not accepting Accept for %d, current promised proposal=%d", args.Ballot, n.promisedBallot)
		return nil
	}

	if existing, ok := n.log[args.Slot]; ok {
		if existing.Ballot.GreaterThan(args.Ballot) {
			reply.Accepted = false
			reply.PromisedBallot = n.promisedBallot
			return nil
		}
	}

	err := n.wal.Append(WalRecord{
		Type:   WalAccept,
		Slot:   args.Slot,
		Ballot: args.Ballot,
		Value:  args.Value,
	})
	if err != nil {
		reply.Accepted = false
		reply.PromisedBallot = n.promisedBallot
		return err
	}
	n.promisedBallot = args.Ballot
	n.log[args.Slot] = AcceptedLogEntry{
		Ballot: args.Ballot,
		Value:  args.Value,
	}

	reply.Accepted = true
	reply.PromisedBallot = n.promisedBallot
	return nil
}

func (n *Node) HandleCommit(args *CommitArgs, reply *CommitReply) error {
	n.mu.Lock()

	if args.LeaderBallot.LessThan(n.promisedBallot) {
		reply.Ok = false
		n.mu.Unlock()
		return nil
	}

	if args.CommitIndex > n.commitIndex {
		err := n.wal.Append(WalRecord{Type: WalCommit, CommitIndex: args.CommitIndex})
		if err != nil {
			reply.Ok = false
			n.mu.Unlock()
			return err
		}
		n.commitIndex = args.CommitIndex
		n.applyCond.Signal()
	}
	n.mu.Unlock()
	reply.Ok = true
	return nil
}

func (n *Node) ApplierLoop() {
	for {
		n.mu.Lock()

		for n.lastApplied >= n.commitIndex {
			n.applyCond.Wait()
		}

		next := n.lastApplied + 1
		_, ok := n.log[next]
		if !ok {
			n.applyCond.Wait()
			n.mu.Unlock()
			continue
		}

		n.mu.Unlock()

		n.apply(next)

		n.mu.Lock()
		n.lastApplied = next
		n.mu.Unlock()
	}
}

func (n *Node) apply(idx int) {
	command := n.log[idx].Value
	n.kv.ApplyPut(command.Key, command.Value)
}

func (n *Node) heartbeatLoop() {
	ticker := time.NewTicker(n.heartbeatInterval)
	defer ticker.Stop()

	for range ticker.C {
		n.mu.Lock()
		if !n.isLeader {
			n.mu.Unlock()
			return
		}

		args := HeartbeatArgs{
			LeaderBallot: n.leaderBallot,
			LeaderID:     n.ID,
			CommitIndex:  n.commitIndex,
		}
		n.mu.Unlock()

		for _, peer := range n.Peers {
			CallHeartbeat(peer, &args)
		}
	}
}

func (n *Node) ElectionLoop() {
	log.Printf("Starting Election loop")
	ticker := time.NewTicker(time.Millisecond * time.Duration(rand.Intn(200)+300))
	defer ticker.Stop()

	for range ticker.C {
		n.mu.Lock()

		if n.isLeader || n.recovering {
			n.mu.Unlock()
			continue
		}

		if time.Since(n.lastHeartbeat[n.leaderBallot.NodeID]) > n.electionTimeout {
			n.lastHeartbeat[n.leaderBallot.NodeID] = time.Now() // prevents election storm
			n.mu.Unlock()
			log.Printf("Election lost heartbeat from %s", n.leaderBallot)
			log.Println("Starting my own Leader Election")
			go n.RunPrepare()
		} else {
			n.mu.Unlock()
		}
	}
}

func (n *Node) HandleHeartbeat(args *HeartbeatArgs, reply *HeartbeatReply) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.LeaderBallot.LessThan(n.promisedBallot) || args.LeaderID != args.LeaderBallot.NodeID {
		reply.Ok = false
		return nil
	}

	if args.LeaderBallot.GreaterThan(n.promisedBallot) {
		n.promisedBallot = args.LeaderBallot
		n.leaderBallot = args.LeaderBallot
	}

	n.isLeader = false

	n.lastHeartbeat[args.LeaderBallot.NodeID] = time.Now()

	if args.CommitIndex > n.commitIndex {
		n.commitIndex = args.CommitIndex
	}

	reply.Ok = true
	return nil
}

func (n *Node) Status() string {
	n.mu.Lock()
	defer n.mu.Unlock()

	return fmt.Sprintf(
		"Node %d\n"+
			"  isLeader: %v\n"+
			"  currentBallot: (%d, %d)\n"+
			"  promisedBallot: (%d, %d)\n"+
			"  peers: %v\n"+
			"  store: %v\n"+
			"  commitIndex: %d\n"+
			"  nextSlot: %d\n"+
			"  lastApplied: %d\n",
		n.ID,
		n.isLeader,
		n.currentBallot.Number, n.currentBallot.NodeID,
		n.promisedBallot.Number, n.promisedBallot.NodeID,
		n.Peers,
		n.kv,
		n.commitIndex,
		n.nextSlot,
		n.lastApplied,
	)
}

func (n *Node) PrintKV() {
	fmt.Println(n.kv.Snapshot())
}

func (n *Node) PrintLog() {
	for i, record := range n.log {
		fmt.Printf("%d-%v\n", i, record)
	}
}
