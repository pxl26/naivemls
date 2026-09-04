// Inspired by LeetCode 297: Serialize and Deserialize Binary Tree
// https://leetcode.com/problems/serialize-and-deserialize-binary-tree/description/

package ratchettree

import (
	"bufio"
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

var dbFile = "mls_db.txt"

func LoadFromDisk() (*MLSTree, error) {
	f, err := os.Open(dbFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	strBuf := strings.Builder{}
	reader := bufio.NewReader(f)
	line, err := reader.ReadString('\n')
	localEpoch, err := strconv.Atoi(line)
	if err != nil {
		return nil, err
	}
	ratchetTree := &MLSTree{Epoch: localEpoch}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		strBuf.WriteString(line)
	}
	ratchetTree.Root = deserialize(strBuf.String())
	return ratchetTree, nil
}
func WriteToDisk(tree *MLSTree) error {
	data := serialize(tree.Root)
	epoch := strconv.Itoa(tree.Epoch)
	data = epoch + "\n" + data
	err := os.WriteFile(dbFile, []byte(data), 0666)
	if err != nil {
		return err
	}
	return nil
}

// Serializes a tree to a single string.
func serialize(root *TreeNode) string {
	if root == nil {
		return "."
	}
	nodeData, err := json.Marshal(root.Val)
	if err != nil {
		panic(err)
	}
	return string(nodeData) + "\n" + serialize(root.Left) + "\n" + serialize(root.Right)
}

// Deserializes your encoded data to tree.
func deserialize(data string) *TreeNode {
	a := strings.Split(data, "\n")
	root, _ := dfs(a, 0)
	return root
}

func dfs(a []string, idx int) (*TreeNode, int) {
	if idx >= len(a) || a[idx] == "." {
		return nil, idx + 1
	}
	var nodeData NodeData
	if err := json.Unmarshal([]byte(a[idx]), &nodeData); err != nil {
		panic(err)
	}
	root := &TreeNode{Val: &nodeData}
	root.Left, idx = dfs(a, idx+1)
	root.Right, idx = dfs(a, idx)
	return root, idx
}
