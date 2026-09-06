package ratchettree

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"

	"github.com/google/uuid"
)

type EncryptedMessage struct {
	Ciphertext []byte
	Nonce      []byte
}

type MLS interface {
	AddMe(history []*Commit) (*Commit, error)
	AddMember(val *NodeData) (*Commit, error)
	RemoveMember(memberNodeID string) (*Commit, error)
	ApplyAddMemberCommit(commit *Commit) error
	ApplyRemoveMemberCommit(commit *Commit) error
	EncryptMessage(plaintext []byte) ([]byte, []byte, error)
	DecryptMessage(nonce []byte, ciphertext []byte) ([]byte, error)
}

type SDK struct {
	tree     *MLSTree
	myNodeID string
}

var _ MLS = (*SDK)(nil)

func (s *SDK) epochCipher() (cipher.AEAD, error) {
	epochSecret := s.tree.Root.Val.Secret
	block, err := aes.NewCipher(epochSecret)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func InitSDK(dbFileName string) (*SDK, error) {
	tree, err := LoadFromDisk(dbFileName)
	if err != nil {
		return nil, err
	}
	if tree.Root == nil {
		tree.AddMember(&NodeData{NodeID: uuid.NewString()})
		if err := PersistToDisk(tree); err != nil {
			tree.DB.Close()
			return nil, err
		}
	}
	leaf := tree.getMyLeafNode()

	return &SDK{
		tree:     tree,
		myNodeID: leaf.Val.NodeID,
	}, nil
}

// Return leaf node belongs SDK owner
func (s *SDK) MemberInfo() *NodeData {
	leaf, _ := findNodeAndParent(s.tree.Root, s.myNodeID)
	return &NodeData{
		NodeID:    s.myNodeID,
		PublicKey: leaf.Val.PublicKey,
	}
}

// Self-add to a group (external join)
// Use latest tree form as GroupInfo to re-build tree locally
// Add himself to group, then inform to group by commit
// Note: in case a new group, pass nil to this func
// TODO: use groupInfo instead of commit histories
func (s *SDK) AddMe(history []*Commit) (*Commit, error) {
	var root *TreeNode
	if len(history) > 0 {
		newestCommit := history[len(history)-1]
		root = deserializeRatchetTree(newestCommit.TreeSkeleton)
	}

	next := &MLSTree{Root: root, Epoch: len(history)}
	commit := next.AddMember(&NodeData{NodeID: s.myNodeID})
	if err := s.save(CloneForMember(next.Root, s.myNodeID), next.Epoch+1); err != nil {
		return nil, err
	}
	return commit, nil
}

// TODO: This func is unused now, this func have to return a "Welcome message" to send to new member
// In "real" MLS, NodeData only contain public key, that called "key package"
func (s *SDK) AddMember(val *NodeData) (*Commit, error) {
	next := &MLSTree{
		Root:  CloneForMember(s.tree.Root, s.myNodeID),
		Epoch: s.tree.Epoch,
	}
	commit := next.AddMember(&NodeData{NodeID: val.NodeID})
	leaf, parent := findNodeAndParent(next.Root, val.NodeID)
	leaf.Val = &NodeData{NodeID: val.NodeID, PublicKey: append([]byte(nil), val.PublicKey...)}
	eph, ciphertext, err := HPKE_Encrypt(val.PublicKey, parent.Val.Secret)
	if err != nil {
		return nil, err
	}
	commit.SecretSyncDatas = append(commit.SecretSyncDatas, &SecretSyncData{NodeID: val.NodeID, EphemeralPubKey: eph, Ciphertext: ciphertext})
	commit.TreeSkeleton = next.GenerateGroupInfo()
	return commit, s.save(CloneForMember(next.Root, s.myNodeID), next.Epoch+1)
}

func (s *SDK) save(root *TreeNode, epoch int) error {
	next := &MLSTree{Root: root, Epoch: epoch, DB: s.tree.DB}
	if err := PersistToDisk(next); err != nil {
		return err
	}
	s.tree = next
	return nil
}

func (s *SDK) RemoveMember(memberNodeID string) (*Commit, error) {
	next := &MLSTree{
		Root:  CloneForMember(s.tree.Root, s.myNodeID),
		Epoch: s.tree.Epoch,
	}
	commit, err := next.RemoveMemberWithCommit(memberNodeID, s.myNodeID)
	if err != nil {
		return nil, err
	}
	return commit, s.save(next.Root, next.Epoch+1)
}

func (s *SDK) apply(commit *Commit, kind CommitType) error {
	next := &MLSTree{Root: CloneForMember(s.tree.Root, s.myNodeID), Epoch: s.tree.Epoch}
	if commit != nil && kind == CommitTypeAddMember && commit.TargetLeafID == s.myNodeID && next.Root.IsLeaf() {
		next.Epoch = int(commit.Epoch)
	}
	if err := next.handleCommit(commit); err != nil {
		return err
	}
	return s.save(next.Root, next.Epoch)
}

func (s *SDK) ApplyAddMemberCommit(commit *Commit) error {
	return s.apply(commit, CommitTypeAddMember)
}

func (s *SDK) ApplyRemoveMemberCommit(commit *Commit) error {
	return s.apply(commit, CommitTypeRemoveMember)
}

func (s *SDK) EncryptMessage(plaintext []byte) ([]byte, []byte, error) {
	gcm, err := s.epochCipher()
	if err != nil {
		return nil, nil, err
	}
	// random a nonce to "Seal"
	// nonce's purpose same with Ephemeral Public Key: one time decrypt/encrypt
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, nil), nil
}

func (s *SDK) DecryptMessage(nonce, ciphertext []byte) ([]byte, error) {
	gcm, err := s.epochCipher()
	if err != nil {
		return nil, err
	}

	return gcm.Open(nil, nonce, ciphertext, nil)
}
