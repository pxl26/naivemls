/*
TODO: consider to use array-based tree representation
https://datatracker.ietf.org/doc/rfc9420/#:~:text=for%20unmerged%20leaves).-,Appendix%20C.%20%20Array%2DBased%20Trees,-One%20benefit%20of
*/
package ratchettree

type TreeNode struct {
	Val   any // Val is nil => empty node
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
	Root *TreeNode
}

/*
https://datatracker.ietf.org/doc/html/rfc9420

	   "Identify the leaf L for the new member:
		if there are empty leaves in the tree, L is the leftmost empty leaf.
		Otherwise, the tree is extended to the right as described in Section 7.7,
		and L is assigned the leftmost new blank leaf."
*/
func (t *MLSTree) AddMember(val any) {
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

// index of leaf node count from left: 0, 1, 2, 3...
// https://datatracker.ietf.org/doc/html/rfc9420#section-7.7
func (t *MLSTree) RemoveMember(index int) {
	curIdx := 0
	var dfs func(cur *TreeNode) bool
	dfs = func(cur *TreeNode) bool {
		if cur == nil {
			return false
		}
		if cur.IsLeaf() {
			if curIdx == index {
				blankNode(cur)
				return true
			}
			curIdx++
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
	dfs(t.Root)
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
