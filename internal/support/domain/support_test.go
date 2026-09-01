package domain

import "testing"

func TestTicketWorkflowRejectsImpossibleTransitions(t *testing.T) {
	cases := []struct {
		from, to string
		allowed  bool
	}{
		{StatusNew, StatusOpen, true},
		{StatusOpen, StatusPendingCustomer, true},
		{StatusPendingCustomer, StatusOpen, true},
		{StatusResolved, StatusClosed, true},
		{StatusClosed, StatusNew, false},
		{StatusNew, StatusResolved, false},
	}
	for _, test := range cases {
		if got := CanTransition(test.from, test.to); got != test.allowed {
			t.Errorf("CanTransition(%q,%q)=%v, want %v", test.from, test.to, got, test.allowed)
		}
	}
}

func TestClosedTicketCannotTransitionOrAcceptReply(t *testing.T) {
	if CanTransition(StatusClosed, StatusPendingCustomer) {
		t.Fatal("closed ticket must not reopen through an agent reply")
	}
	if CanTransition(StatusClosed, StatusNew) {
		t.Fatal("closed ticket must not return to new")
	}
}
