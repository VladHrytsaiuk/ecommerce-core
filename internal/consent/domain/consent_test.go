package domain

import "testing"

func TestErasureTopicIsVersionedAndMinimal(t *testing.T) {
	if TopicErasureRequested != "privacy.erasure_requested.v1" {
		t.Fatalf("topic = %q", TopicErasureRequested)
	}
}
