package ratchettree

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const treeDiagramMarker = "\n\n# Ratchet tree\n"

func LoadFromDisk(path string) (*MLSTree, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	// keep file open to write later
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	tree := &MLSTree{DB: *file}
	if len(data) != 0 {
		epochLine, treeData, ok := strings.Cut(string(data), "\n")
		if !ok {
			return nil, fmt.Errorf("malformal file structures")
		}

		tree.Epoch, err = strconv.Atoi(strings.TrimSpace(epochLine))
		if err != nil || tree.Epoch < 0 {
			return nil, fmt.Errorf("invalid epoch in DB: %q", epochLine)
		}

		// ignore render tree at the end of file
		treeData, _, _ = strings.Cut(treeData, treeDiagramMarker)
		tree.Root = deserializeRatchetTree(treeData)
	}
	return tree, nil
}

func PersistToDisk(tree *MLSTree) error {
	data := strconv.Itoa(tree.Epoch) + "\n" + serialize(tree.Root)
	data += treeDiagramMarker + renderTree(tree.Root)

	// TODO: consider re-open file

	// Delete all existed content in file
	if err := tree.DB.Truncate(0); err != nil {
		return err
	}

	// Move pointer to the beginning of file
	if _, err := tree.DB.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// Write new data to file
	if _, err := tree.DB.Write([]byte(data)); err != nil {
		return err
	}

	return tree.DB.Sync()
}

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

func serializeWithoutPrivateKey(root *TreeNode) string {
	if root == nil {
		return "."
	}
	var publicData *NodeData
	if root.Val != nil {
		copy := *root.Val // clone node to avoid impact original node
		copy.PrivateKey = nil
		copy.Secret = nil
		publicData = &copy
	}
	nodeData, err := json.Marshal(publicData)
	if err != nil {
		panic(err)
	}
	return string(nodeData) + "\n" + serializeWithoutPrivateKey(root.Left) + "\n" + serializeWithoutPrivateKey(root.Right)
}

func deserializeRatchetTree(data string) *TreeNode {
	if len(data) == 0 {
		return nil
	}
	a := strings.Split(data, "\n")
	root, _ := dfs(a, 0)
	return root
}

func dfs(a []string, idx int) (*TreeNode, int) {
	if idx >= len(a) || a[idx] == "." {
		return nil, idx + 1
	}
	root := &TreeNode{}
	if a[idx] != "null" {
		var nodeData NodeData
		if err := json.Unmarshal([]byte(a[idx]), &nodeData); err != nil {
			panic(err)
		}
		root.Val = &nodeData
	}
	root.Left, idx = dfs(a, idx+1)
	root.Right, idx = dfs(a, idx)
	return root, idx
}

// renderTree shows topology and key ownership without repeating key material.
// note: AI making
func renderTree(root *TreeNode) string {
	var out strings.Builder
	var walk func(*TreeNode, string, string, string)
	walk = func(node *TreeNode, prefix, branch, position string) {
		label := "(nil)"
		if node != nil {
			label = "(blank)"
			if node.Val != nil {
				kind := "node"
				if node.IsLeaf() {
					kind = "leaf"
				}
				label = fmt.Sprintf("%s [%s, private=%t]", node.Val.NodeID, kind, len(node.Val.PrivateKey) > 0)
			}
		}
		fmt.Fprintf(&out, "%s%s%s: %s\n", prefix, branch, position, label)
		if node == nil || node.IsLeaf() {
			return
		}
		childPrefix := prefix
		if branch == "├── " {
			childPrefix += "│   "
		} else if branch == "└── " {
			childPrefix += "    "
		}
		walk(node.Left, childPrefix, "├── ", "L")
		walk(node.Right, childPrefix, "└── ", "R")
	}
	walk(root, "", "", "root")
	return out.String()
}
