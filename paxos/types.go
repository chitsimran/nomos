package paxos

type AcceptedLogEntry struct {
	Ballot Ballot
	Value  Command
}

type Command struct {
	Key   string
	Value string
}

type WalRecordType int

const (
	WalPromise WalRecordType = iota
	WalAccept
	WalCommit
)

type WalRecord struct {
	Type WalRecordType

	PromisedBallot Ballot

	Slot   int
	Ballot Ballot
	Value  Command

	CommitIndex int
}
