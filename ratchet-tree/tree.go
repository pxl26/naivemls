/*
TODO: consider to use array-based tree representation
https://datatracker.ietf.org/doc/rfc9420/#:~:text=for%20unmerged%20leaves).-,Appendix%20C.%20%20Array%2DBased%20Trees,-One%20benefit%20of
*/
package ratchettree

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"

	"github.com/google/uuid"
)

type SecretSyncData struct {
	NodeID          string
	EphemeralPubKey []byte
	Ciphertext      []byte
}

type NodeData struct {
	NodeID     string
	PublicKey  []byte
	PrivateKey []byte
	Secret     []byte
}

type TreeNode struct {
	Val *NodeData

	Left  *TreeNode
	Right *TreeNode
}

func (t *TreeNode) IsLeaf() bool {
	return t.Left == nil && t.Right == nil
}

func (t *TreeNode) IsBlank() bool {
	return t.Val == nil
}

type MLSTree struct {
	Root  *TreeNode
	Epoch int

	DB os.File
}

func (t *MLSTree) AddMember(val *NodeData) *Commit {
	if val == nil {
		panic("member data is required")
	}

	t.addTreeNode(val)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(err)
	}
	secretSyncDatas := refreshKeyAlongPath(t.Root, secret, val.NodeID)
	return &Commit{
		Epoch:           uint64(t.Epoch),
		Type:            CommitTypeAddMember,
		TargetLeafID:    val.NodeID,
		TreeSkeleton:    t.GenerateGroupInfo(),
		SecretSyncDatas: secretSyncDatas,
	}
}

func (t *MLSTree) HandleAddCommit(commit *Commit) error {
	return t.handleCommit(commit)
}

func (t *MLSTree) HandleRemoveCommit(proposal *Commit) error {
	return t.handleCommit(proposal)
}

func (t *MLSTree) handleCommit(commit *Commit) error {
	if t.Epoch < 0 || commit.Epoch != uint64(t.Epoch) {
		return errors.New("invalid commit epoch")
	}

	newRoot := deserializeRatchetTree(commit.TreeSkeleton)

	// collect privateKey from current tree
	// then merge them to new tree from commit
	localKeys := collectPrivateNodes(t.Root)
	mergePrivateNodes(newRoot, localKeys)

	// The member that created the proposal may already have applied the new
	// direct path locally in AddMember. In that case the matching private root
	// key was safely merged and no path secret needs to be decrypted again.
	if newRoot.Val != nil && len(newRoot.Val.PrivateKey) > 0 {
		t.Root = newRoot
		t.Epoch++
		return nil
	}

	// looking for secret be send to me
	// my node is a node contains privateKey
	for _, syncData := range commit.SecretSyncDatas {
		if syncData == nil {
			continue
		}
		localNode, ok := localKeys[syncData.NodeID]
		if !ok || len(localNode.PrivateKey) == 0 { // local node has privateKey
			continue
		}
		secret, err := HPKE_Decrypt(localNode.PrivateKey, syncData.EphemeralPubKey, syncData.Ciphertext)
		if err != nil {
			continue
		}

		// generate secret from parent to root
		_, parent := findNodeAndParent(newRoot, syncData.NodeID)
		if parent == nil {
			continue
		}
		if err := applyPathSecret(newRoot, parent, secret); err != nil {
			return err
		}

		t.Root = newRoot
		t.Epoch++
		return nil
	}

	return errors.New("Cannot found any SecretSyncData be send to me")
}

func (t *MLSTree) RemoveMemberWithCommit(memberNodeID, committerNodeID string) (*Commit, error) {
	if !t.RemoveMember(memberNodeID) {
		return nil, errors.New("Remove member fail")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	secretSyncDatas := refreshKeyAlongPath(t.Root, secret, committerNodeID)
	return &Commit{
		Epoch:           uint64(t.Epoch),
		Type:            CommitTypeRemoveMember,
		TargetLeafID:    memberNodeID,
		TreeSkeleton:    t.GenerateGroupInfo(),
		SecretSyncDatas: secretSyncDatas,
	}, nil
}

func collectPrivateNodes(root *TreeNode) map[string]NodeData {
	nodes := make(map[string]NodeData)
	var dfs func(*TreeNode)
	dfs = func(node *TreeNode) {
		if node == nil {
			return
		}
		if node.Val != nil && node.Val.NodeID != "" && len(node.Val.PrivateKey) > 0 {
			nodes[node.Val.NodeID] = *node.Val
		}
		dfs(node.Left)
		dfs(node.Right)
	}
	dfs(root)
	return nodes
}

func mergePrivateNodes(root *TreeNode, localNodes map[string]NodeData) {
	if root == nil {
		return
	}
	if root.Val != nil {
		// ensure same public key
		// because may keypair of this node was rotated before
		if local, ok := localNodes[root.Val.NodeID]; ok && bytes.Equal(local.PublicKey, root.Val.PublicKey) {
			root.Val.PrivateKey = append([]byte(nil), local.PrivateKey...)
			root.Val.Secret = append([]byte(nil), local.Secret...)
		}
	}
	mergePrivateNodes(root.Left, localNodes)
	mergePrivateNodes(root.Right, localNodes)
}

func findNodeAndParent(root *TreeNode, nodeID string) (*TreeNode, *TreeNode) {
	var dfs func(*TreeNode, *TreeNode) (*TreeNode, *TreeNode)
	dfs = func(node, parent *TreeNode) (*TreeNode, *TreeNode) {
		if node == nil {
			return nil, nil
		}
		if node.Val != nil && node.Val.NodeID == nodeID {
			return node, parent
		}
		if found, foundParent := dfs(node.Left, node); found != nil {
			return found, foundParent
		}
		return dfs(node.Right, node)
	}
	return dfs(root, nil)
}

func applyPathSecret(root, first *TreeNode, secret []byte) error {
	current := first
	for current != nil {
		if current.Val == nil {
			return errors.New("direct-path node has no public data")
		}
		privateKey, _ := GenerateKeypair(secret) // public key has already exist in tree

		current.Val.PrivateKey = privateKey
		current.Val.Secret = append([]byte(nil), secret...)
		if current == root {
			return nil
		}
		_, parent := findParent(root, current)
		current = parent
		secret = DeriveParentSecret(secret)
	}
	return errors.New("direct path does not reach the root")
}

func findParent(root, target *TreeNode) (*TreeNode, *TreeNode) {
	if root == nil || target == nil {
		return nil, nil
	}
	if root.Left == target || root.Right == target {
		return target, root
	}
	if found, parent := findParent(root.Left, target); found != nil {
		return found, parent
	}
	return findParent(root.Right, target)
}

// There are 3 tasks here:
// 1. Blank nodes on the path from leafID to root
// 2. Generate new keyPair for nodes on the path from leafID to root by KDF(path_secret)
// 3. Send secret to others user by send secret via copath nodes
func refreshKeyAlongPath(root *TreeNode, leafSecret []byte, leafID string) []*SecretSyncData {
	secretSyncDatas := []*SecretSyncData{}
	var dfs func(*TreeNode) []byte
	dfs = func(cur *TreeNode) []byte {
		if cur == nil {
			return nil
		}
		if cur.IsLeaf() {
			if cur.Val != nil && cur.Val.NodeID == leafID {
				cur.Val.PrivateKey, cur.Val.PublicKey = GenerateKeypair(leafSecret)
				cur.Val.Secret = leafSecret
				return DeriveParentSecret(leafSecret)
			}
			return []byte{}
		}
		secret := dfs(cur.Left)
		copath := cur.Right

		if len(secret) == 0 {
			copath = cur.Left
			secret = dfs(cur.Right)
		}
		if len(secret) == 0 {
			return nil
		}
		targetPubKeys := resolveCopath(copath)
		for _, targetPubKey := range targetPubKeys {
			ephPub, ciphertext, err := HPKE_Encrypt(targetPubKey.PublicKey, secret)
			if err != nil {
				panic(err)
			}
			secretSyncDatas = append(secretSyncDatas, &SecretSyncData{
				NodeID:          targetPubKey.NodeID,
				EphemeralPubKey: ephPub,
				Ciphertext:      ciphertext,
			})
		}

		if cur.Val == nil {
			cur.Val = &NodeData{NodeID: uuid.NewString()}
		}
		cur.Val.PrivateKey, cur.Val.PublicKey = GenerateKeypair(secret)
		cur.Val.Secret = secret
		return DeriveParentSecret(secret)
	}
	dfs(root)
	return secretSyncDatas
}

// find all non-blank nodes in a subtree
func resolveCopath(node *TreeNode) []*NodeData {
	if node == nil {
		return nil
	}
	if !node.IsBlank() {
		return []*NodeData{node.Val}
	}

	var keys []*NodeData
	keys = append(keys, resolveCopath(node.Left)...)
	keys = append(keys, resolveCopath(node.Right)...)
	return keys
}

/*
https://datatracker.ietf.org/doc/html/rfc9420

	   "Identify the leaf L for the new member:
		if there are empty leaves in the tree, L is the leftmost empty leaf.
		Otherwise, the tree is extended to the right as described in Section 7.7,
		and L is assigned the leftmost new blank leaf."
*/
func (t *MLSTree) addTreeNode(val *NodeData) {
	if t.Root == nil {
		t.Root = &TreeNode{Val: val}
		return
	}
	// if there are empty leaves in the tree, L is the leftmost empty leaf
	node := mostLeftBlankLeafNode(t.Root)
	if node != nil {
		node.Val = val
		return
	}
	// the tree is extended to the right -> L is assigned the leftmost new blank leaf.
	newRoot := &TreeNode{}
	newRoot.Left = t.Root
	newRoot.Right = &TreeNode{Val: val}
	t.Root = newRoot
}

func mostLeftBlankLeafNode(cur *TreeNode) *TreeNode {
	if cur == nil {
		return nil
	}
	if cur.IsLeaf() && cur.IsBlank() {
		return cur
	}

	if node := mostLeftBlankLeafNode(cur.Left); node != nil {
		return node
	}
	return mostLeftBlankLeafNode(cur.Right)
}

// RemoveMember blanks the leaf identified by memberNodeID and its direct path.
// It returns false when memberNodeID is not an active leaf member.
func (t *MLSTree) RemoveMember(memberNodeID string) bool {
	var dfs func(cur *TreeNode) bool
	dfs = func(cur *TreeNode) bool {
		if cur == nil {
			return false
		}
		if cur.IsLeaf() {
			if cur.Val != nil && cur.Val.NodeID == memberNodeID {
				blankNode(cur)
				return true
			}
			return false
		}
		if dfs(cur.Left) {
			blankNode(cur)
			return true
		}
		if dfs(cur.Right) {
			blankNode(cur)
			return true
		}
		return false
	}
	return dfs(t.Root)
}

func (t *MLSTree) GenerateGroupInfo() string {
	return serializeWithoutPrivateKey(t.Root)
}

// Is a leaf node contains private key (only leaf owner has private key)
func (t *MLSTree) getMyLeafNode() *TreeNode {
	var dfs func(cur *TreeNode) *TreeNode
	dfs = func(cur *TreeNode) *TreeNode {
		if cur == nil {
			return nil
		}
		if cur.IsLeaf() && cur.Val != nil && len(cur.Val.PrivateKey) > 0 {
			return cur
		}
		if node := dfs(cur.Left); node != nil {
			return node
		}
		return dfs(cur.Right)
	}
	return dfs(t.Root)
}

func blankNode(node *TreeNode) {
	node.Val = nil
}

func lowestCommonAncestor(root *TreeNode, key1, key2 any) *TreeNode {
	if root == nil || root.Val == key1 || root.Val == key2 {
		return root
	}
	left := lowestCommonAncestor(root.Left, key1, key2)
	right := lowestCommonAncestor(root.Right, key1, key2)
	if left != nil && right != nil {
		return root
	}
	if left != nil {
		return left
	}
	return right
}

// Check may if exist a LEAF node has nodeID == userID
func HasMember(root *TreeNode, userID string) bool {
	if root == nil {
		return false
	}
	if root.IsLeaf() {
		return root.Val != nil && root.Val.NodeID == userID
	}
	return HasMember(root.Left, userID) || HasMember(root.Right, userID)
}

// Create a copy of tree, but "opaque" private key on nodes which not on the path of userID
// UserID is nodeID
func CloneForMember(root *TreeNode, userID string) *TreeNode {
	if root == nil {
		return nil
	}
	node := &TreeNode{
		Left:  CloneForMember(root.Left, userID),
		Right: CloneForMember(root.Right, userID),
	}
	if root.Val != nil {
		value := *root.Val
		value.PublicKey = append([]byte(nil), root.Val.PublicKey...) // same with: slices.Clone(root.Val.PublicKey)
		value.PrivateKey = append([]byte(nil), root.Val.PrivateKey...)
		value.Secret = append([]byte(nil), root.Val.Secret...)
		if !HasMember(root, userID) {
			value.PrivateKey = nil
			value.Secret = nil
		}
		node.Val = &value
	}
	return node
}
