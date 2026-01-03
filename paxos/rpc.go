package paxos

type PrepareArgs struct {
	Ballot Ballot
}

type PrepareReply struct {
	Promised       bool
	PromisedBallot Ballot

	Accepted map[int]AcceptedLogEntry
}

type AcceptArgs struct {
	Ballot Ballot
	Slot   int
	Value  Command
}

type AcceptReply struct {
	Accepted       bool
	PromisedBallot Ballot
}

type CommitArgs struct {
	LeaderBallot Ballot
	CommitIndex  int
}

type CommitReply struct {
	Ok bool
}

type HeartbeatArgs struct {
	LeaderBallot Ballot
	LeaderID     int
	CommitIndex  int
}

type HeartbeatReply struct {
	Ok bool
}
