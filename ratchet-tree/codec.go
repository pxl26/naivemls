// Inspired by LeetCode 297: Serialize and Deserialize Binary Tree
// https://leetcode.com/problems/serialize-and-deserialize-binary-tree/description/

package ratchettree

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var dbFile = "mls_db.txt"

func LoadFromDisk() (*TreeNode, error) {
	f, err := os.Open(dbFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	strBuf := strings.Builder{}
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		strBuf.WriteString(line)
	}
	return deserialize(strBuf.String()), nil
}
func WriteToDisk(root *TreeNode) error {
	data := serialize(root)
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
	rs := fmt.Sprint(root.Val)
	return rs + "\n" + serialize(root.Left) + "\n" + serialize(root.Right)
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
	root := &TreeNode{Val: atoi(a[idx])}
	root.Left, idx = dfs(a, idx+1)
	root.Right, idx = dfs(a, idx)
	return root, idx
}

func atoi(s string) int {
	rs, _ := strconv.Atoi(s)
	return rs
}
