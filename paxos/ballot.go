package paxos

// Ballot defines a totally ordered Paxos ballot.
// Ballots are compared lexicographically by (Number, NodeID)
// and must only increase over time.
type Ballot struct {
	Number int
	NodeID int
}

func (b Ballot) GreaterThan(other Ballot) bool {
	if b.Number == other.Number {
		return b.NodeID > other.NodeID
	}
	return b.Number > other.Number
}

func (b Ballot) GreaterThanOrEqualTo(other Ballot) bool {
	return b.Equal(other) || b.GreaterThan(other)
}

func (b Ballot) LessThan(other Ballot) bool {
	return other.GreaterThan(b)
}

func (b Ballot) Equal(other Ballot) bool {
	return b.Number == other.Number && b.NodeID == other.NodeID
}

func (b *Ballot) Increment(nodeID int) {
	b.Number++
	b.NodeID = nodeID
}
