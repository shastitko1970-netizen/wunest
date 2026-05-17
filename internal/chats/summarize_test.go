package chats

import "testing"

func msg(id int64, role Role, content string) Message {
	return Message{ID: id, Role: role, Content: content}
}

func TestPickSummariserBounds_forceRefreshWhenCovered(t *testing.T) {
	history := []Message{
		msg(1, RoleUser, "hi"),
		msg(2, RoleAssistant, "hello"),
		msg(3, RoleUser, "bye"),
	}
	toFold, _ := PickSummariserBounds(history, 3, true)
	if len(toFold) != 0 {
		t.Fatalf("expected no incremental fold when all covered, got %d", len(toFold))
	}
	all := PickForceRefreshMessages(history)
	if len(all) != 3 {
		t.Fatalf("force refresh: want 3 messages, got %d", len(all))
	}
}

func TestPickAutoSummariserBounds_shortChat(t *testing.T) {
	history := make([]Message, 0, 12)
	for i := 0; i < 12; i++ {
		role := RoleUser
		if i%2 == 1 {
			role = RoleAssistant
		}
		history = append(history, msg(int64(i+1), role, "x"))
	}
	toFold, _ := PickSummariserBounds(history, 0, false)
	if len(toFold) != 0 {
		t.Fatalf("normal auto bounds should be empty for 12 msgs, got %d", len(toFold))
	}
	toFold, keep := PickAutoSummariserBounds(history, 0)
	if len(toFold) != 7 { // 12 - autoKeepRecentFloor(5)
		t.Fatalf("auto bounds: want 7 folded, got %d (keep=%d)", len(toFold), keep)
	}
	if keep != 7 {
		t.Fatalf("auto keep idx: want 7 (12-5), got %d", keep)
	}
}

func TestEstimatePromptTokensFromHistory(t *testing.T) {
	history := []Message{
		msg(1, RoleUser, string(make([]byte, 4000))),
	}
	got := estimatePromptTokensFromHistory(history)
	if got < 900 || got > 1100 {
		t.Fatalf("estimate: want ~1000, got %d", got)
	}
}
