package main

import (
	"fmt"
	"os"
	"testing"

	ratchettree "naive-mls/ratchet-tree"
)

func TestMLS(t *testing.T) {
	fmt.Printf("Test %d", 999999)
	initSDK := func(name string) *ratchettree.SDK {
		path := fmt.Sprintf("db_mls_%s.txt", name)
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		sdk, err := ratchettree.InitSDK(path)
		if err != nil {
			t.Fatal(err)
		}
		return sdk
	}
	alice := initSDK("alice")
	bob := initSDK("bob")
	floyd := initSDK("floyd")

	commit1, err := alice.AddMe(nil)
	if err != nil {
		t.Fatalf("Alice AddMe: %v", err)
	}
	commit2, err := bob.AddMe([]*ratchettree.Commit{commit1})
	if err != nil {
		t.Fatalf("Bob AddMe: %v", err)
	}
	if err := alice.ApplyAddMemberCommit(commit2); err != nil {
		t.Fatal(err)
	}

	// ---------------------------------------- Bob & Alice talk to each other ----------------------------------------

	// Bob --"Hello"--> Alice
	sendMsg := []byte("Hello alice")
	nonce, ciphertext, err := bob.EncryptMessage(sendMsg)
	if err != nil {
		t.Fatal(err)
	}

	receiveMsg, err := alice.DecryptMessage(nonce, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(sendMsg) != string(receiveMsg) {
		t.Fatalf("Alice DecryptMessage: got %q, want %q", string(receiveMsg), string(sendMsg))
	}

	// Alice --"Hi Bob"--> Bob
	sendMsg = []byte("Hi Bob")
	nonce, ciphertext, err = alice.EncryptMessage(sendMsg)
	if err != nil {
		t.Fatal(err)
	}
	receiveMsg, err = bob.DecryptMessage(nonce, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(sendMsg) != string(receiveMsg) {
		t.Fatalf("Bob DecryptMessage: got %q, want %q", string(receiveMsg), string(sendMsg))
	}
	// ---------------------------------------- add Floyd to the group ----------------------------------------
	// Floyd add himself
	commit3, err := floyd.AddMe([]*ratchettree.Commit{commit1, commit2})
	if err != nil {
		t.Fatal(err)
	}
	// Bob apply the commit => Bob can communicate with Floyd
	if err := bob.ApplyAddMemberCommit(commit3); err != nil {
		t.Fatal(err)
	}
	// Floyd --"Hi all"--> Group
	floydSendMsg := []byte("Hi all")
	floydNonce, floydCiphertext, err := floyd.EncryptMessage(floydSendMsg)
	if err != nil {
		t.Fatal(err)
	}
	receiveMsg, err = bob.DecryptMessage(floydNonce, floydCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(floydSendMsg) != string(receiveMsg) {
		t.Fatalf("Bob DecryptMessage: got %q, want %q", string(receiveMsg), string(floydSendMsg))
	}
	// ---------------------------------------- Bob & Alice & Floyd talk to each other ----------------------------------------

	// Bob --"welcom floyd"--> Group
	// Floyd can read
	// Alice can NOT read

	// -- Floyd try to read (OK)
	bobSendMsg := []byte("welcom floyd")
	bobNonce, bobCiphertext, err := bob.EncryptMessage(bobSendMsg)
	if err != nil {
		t.Fatal(err)
	}
	receiveMsg, err = floyd.DecryptMessage(bobNonce, bobCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(bobSendMsg) != string(receiveMsg) {
		t.Fatalf("Floyd DecryptMessage: got %q, want %q", string(receiveMsg), string(bobSendMsg))
	}

	// -- Alice try to read (FAIL because desyn)
	receiveMsg, err = alice.DecryptMessage(bobNonce, bobCiphertext)
	if err == nil {
		t.Fatalf("Alice should not to read Bob msg")
	}
	receiveMsg, err = alice.DecryptMessage(floydNonce, floydCiphertext)
	if err == nil {
		t.Fatalf("Alice should not to read Floyd msg")
	}

	// Alice --"Anyone here?" --> Group
	// No one can read => because Alice is desyn now
	aliceSendMsg := []byte("Anyone here?")
	aliceNonce, aliceCiphertext, err := alice.EncryptMessage(aliceSendMsg)
	if err != nil {
		t.Fatal(err)
	}
	receiveMsg, err = bob.DecryptMessage(aliceNonce, aliceCiphertext)
	if err == nil {
		t.Fatalf("Bob should not to read Alice msg")
	}
	receiveMsg, err = floyd.DecryptMessage(aliceNonce, aliceCiphertext)
	if err == nil {
		t.Fatalf("Floyd should not to read Alice msg")
	}

	// --- Alice apply Floyd's commit (commit3) => Back to sync
	if err := alice.ApplyAddMemberCommit(commit3); err != nil {
		t.Fatal(err)
	}

	// Alice can read Bob's msg
	receiveMsg, err = alice.DecryptMessage(bobNonce, bobCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(bobSendMsg) != string(receiveMsg) {
		t.Fatalf("Alice DecryptMessage: got %q, want %q", string(receiveMsg), string(bobSendMsg))
	}
	// Alice can read Floyd's msg
	receiveMsg, err = alice.DecryptMessage(floydNonce, floydCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(floydSendMsg) != string(receiveMsg) {
		t.Fatalf("Alice DecryptMessage: got %q, want %q", string(receiveMsg), string(floydSendMsg))
	}
	// Alice --"Iam back"--> gay group
	aliceSendMsg = []byte("Iam back")
	aliceNonce, aliceCiphertext, err = alice.EncryptMessage(aliceSendMsg)
	if err != nil {
		t.Fatal(err)
	}
	// Bob and Floyd can read
	receiveMsg, err = bob.DecryptMessage(aliceNonce, aliceCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(aliceSendMsg) != string(receiveMsg) {
		t.Fatalf("Bob DecryptMessage: got %q, want %q", string(receiveMsg), string(aliceSendMsg))
	}
	receiveMsg, err = floyd.DecryptMessage(aliceNonce, aliceCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(aliceSendMsg) != string(receiveMsg) {
		t.Fatalf("Floyd DecryptMessage: got %q, want %q", string(receiveMsg), string(aliceSendMsg))
	}

	// ------- Bob was removed ------
	removeCommit, err := alice.RemoveMember(bob.MemberInfo().NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := floyd.ApplyRemoveMemberCommit(removeCommit); err != nil {
		t.Fatal(err)
	}

	floydSendMsg = []byte("where is Bob???")
	floydNonce, floydCiphertext, err = floyd.EncryptMessage(floydSendMsg)
	if err != nil {
		t.Fatal(err)
	}

	receiveMsg, err = alice.DecryptMessage(floydNonce, floydCiphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(floydSendMsg) != string(receiveMsg) {
		t.Fatalf("Alice DecryptMessage: got %q, want %q", string(receiveMsg), string(floydSendMsg))
	}

	receiveMsg, err = bob.DecryptMessage(floydNonce, floydCiphertext)
	if err == nil {
		t.Fatalf("Bob should not to read Floyd msg")
	}
}

func TestPointer(t *testing.T) {
	type Data struct {
		name string
	}
	origin := &Data{
		name: "Alice",
	}

	clone := *origin
	clone.name = "Bob"
	fmt.Println(origin.name)
}
