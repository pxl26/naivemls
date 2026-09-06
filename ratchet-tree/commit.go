package ratchettree

type CommitType int

const (
	CommitTypeAddMember CommitType = iota
	CommitTypeRemoveMember
	CommitTypeRotateKey
)

type Commit struct {
	Epoch           uint64 // current epoch before update, after apply commit successfuly then increase local epoch
	Type            CommitType
	TargetLeafID    string
	TreeSkeleton    string // tree form with public key only
	SecretSyncDatas []*SecretSyncData
}
