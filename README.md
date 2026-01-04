# nomos
Nomos is a fault-tolerant distributed key-value store implemented using Multi-Paxos and partly inspired by Raft.
It supports leader election, replicated logs, crash-safe persistence via WAL, quorum-based recovery, and linearizable reads and writes. The system tolerates node crashes and restarts while preserving safety and liveness.

## Overview
Nomos provides a key-value store replicated across multiple nodes.
Each node acts as a replicated state machine, ensuring that all nodes apply commands in the same total order.

Client commands are replicated using Multi-Paxos:
- Each log slot corresponds to a Paxos consensus instance 
- Once a command is accepted by a quorum, it is committed 
- Committed commands are applied deterministically to the KV store

Only the leader coordinates replication, which simplifies correctness and ensures linearizability.


## System Properties
Nomos operates in two high-level phases:
1. Leader Election
- Safety: At most one leader is active at any given time
- Liveness: Some node is eventually elected leader

Leader election is driven by ballot-based Prepare rounds and randomized election timeouts to avoid split votes.
2. Command Replication
- Safety: No two nodes ever apply different commands for the same log slot
- Liveness: Logs at all nodes eventually converge

## Design
Nomos enforces a single active leader at any time.
All client reads and writes go through the leader, which ensures linearizability.

### Consensus & Replication

Nomos uses Multi-Paxos to replicate a sequence of commands across nodes.

- Leader election is performed using ballot-based Prepare rounds.
- Each log slot is chosen via quorum-based Accept rounds.
- A slot is considered committed once a majority of nodes accept it.
- Committed entries are applied in order to the state machine.

### Leader Election & Failure Detection

- Leaders periodically send heartbeats to followers 
- Followers track the time since the last heartbeat 
- If a follower does not receive a heartbeat within its randomized election timeout, it initiates a new Prepare phase 
- Randomized election timeouts ensure liveness by preventing repeated split votes

### Recovery & Log Reconstruction

When a node becomes leader, it reconstructs the log by collecting accepted entries from a quorum during the Prepare phase.

Key recovery properties:
- For each slot, the value with the highest ballot is selected 
- Recovered entries are persisted to disk before being applied 
- The leader re-proposes recovered entries to ensure consistency

This ensures that committed commands are never lost, even if the new leader crashes during recovery.

## Node State and key variables
Each node maintains the following important state:
- `log`: Map of log slot - accepted entry (ballot, value)
- `commitIndex`: Highest log slot known to be committed 
- `lastApplied`: Highest log slot applied to the local KV store 
- `nextSlot`: Next log slot the leader will propose
- `currentBallot`: Ballot used when attempting to become leader 
- `promisedBallot`: Highest ballot this node has promised not to violate 
- `leaderBallot`: Ballot of the current known leader

Commands are only applied when:
- they are present in the log 
- their slot index ≤ commitIndex 
- all prior slots have been applied

## Persistence & Durability

Nomos uses a write-ahead log (WAL) to ensure crash safety.

- Promises, accepts, and commits are durably logged before affecting in-memory state.
- On restart, nodes replay the WAL to reconstruct their log and commit index.
- Recovered log entries learned during leader election are also persisted to avoid data loss.

## Read & Write Semantics

- Writes are linearizable and replicated using Multi-Paxos.
- Reads are served by the current leader only.
- Followers reject reads to prevent stale data exposure.
- Leader do not serve reads or writes while recovering.

## Fault Tolerance Guarantees

Nomos tolerates:
- Up to ⌊(N−1)/2⌋ node crashes 
- Leader failure during replication 
- Lagging or temporarily unavailable replicas

Safety is preserved as long as a majority of nodes are available.

## Running Nomos

Start multiple nodes on different ports:

```bash
go run client/main.go --id=1 --port=8001
go run client/main.go --id=2 --port=8002
go run client/main.go --id=3 --port=8003
```

#### Peer Configuration (important)

Peers are currently hard-coded in client/main.go.

To change cluster membership, you must either:
- edit the peer address map in main.go, or 
- extend the code to accept peer addresses via command-line flags

Dynamic membership is not currently supported.

## Interactive Commands

Each node runs an interactive REPL with the following commands:

`prepare` Manually triggers a leader election (Prepare phase) (was mainly used for initial testing)

`put <key> <value>` Submits a write request to the leader

`get <key>` Reads a value from the leader (followers reject)

`status` Prints the node’s internal state (ballots, indices, role)

`kv` Prints the current key-value store contents

`log` Prints the current local log

`ping <addr>` Sends a ping RPC to another node (for connectivity testing)

`exit` Shuts down the node

## Limitations & Future Work

- No log compaction or snapshotting (WAL grows unbounded)
- No explicit follower catch-up RPCs beyond Paxos recovery
- Reads use leader-only semantics (no lease-based or quorum reads)
- Static cluster membership

Future improvements include snapshot installation, log truncation, and batched logging.
