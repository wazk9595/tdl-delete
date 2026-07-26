package main

import (
	"bytes"
	"context"
	"testing"
)

func TestExecuteDeleteDryRunDoesNotUseClient(t *testing.T) {
	var out bytes.Buffer

	err := executeDelete(context.Background(), nil, &out, map[int64][]int{
		20: {3},
		10: {1, 2},
	}, true, true)
	if err != nil {
		t.Fatalf("executeDelete() error = %v", err)
	}
	want := "" +
		"Would delete 2 message(s) from chat 10: 1,2 (revoke=true)\n" +
		"Would delete 1 message(s) from chat 20: 3 (revoke=true)\n" +
		"Dry run: would delete 3 message(s); no messages were deleted\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestDeletePeerMessagesDryRunDoesNotUseClient(t *testing.T) {
	var out bytes.Buffer

	err := deletePeerMessages(context.Background(), nil, &out, "me", []int{73767}, true, true)
	if err != nil {
		t.Fatalf("deletePeerMessages() error = %v", err)
	}
	want := "" +
		"Would delete 1 message(s) from me: 73767 (revoke=true)\n" +
		"Dry run: would delete 1 message(s); no messages were deleted\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
