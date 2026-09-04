package mls

type ProposalType int

const (
	ProposalTypeAddMember ProposalType = iota
	ProposalTypeRemoveMember
	ProposalTypeRotateKey
)

type Proposal struct {
	Epoch uint64
	Type  ProposalType
	// AuthorID      int
	TargetLeafID  int
	Secret        map[int]string // receiver_id -> lowest_blank_node's secret -> KDF to root
	NewPublicKeys []string       // New public keys ordered by TargetLeaf -> root
}
