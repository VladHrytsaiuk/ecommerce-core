package app

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	abandonedApp "github.com/VladHrytsaiuk/ecommerce-core/internal/abandoned_cart/application"
	eventsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
)

// A consumer only ever receives a delivery for a topic it registered a handler
// for. The outbox worker now fails a delivery it cannot handle rather than
// acknowledging it, so a route that outruns its handlers is no longer silent —
// but it is still a misconfiguration, and these tests keep the two in step.

func TestCartTopicsGoOnlyToTheConsumerThatHandlesThem(t *testing.T) {
	// Both consumers used to receive both cart topics, because a plain
	// publisher is blind to the topic and writes a row per listed consumer.
	// Reports has no cart.updated handler and the campaign producer has no
	// carts.created handler, so half of those deliveries were dead weight the
	// worker quietly marked done.
	routes := cartEventRoutes(true, true)

	assertRoutedExactly(t, routes, eventsDomain.TopicCartCreated, eventsDomain.ConsumerReportsProjection)
	assertRoutedExactly(t, routes, eventsDomain.TopicCartUpdated, abandonedApp.ConsumerCampaignProducer)
}

func TestCartRoutesFollowTheEnabledModules(t *testing.T) {
	if routes := cartEventRoutes(false, false); len(routes) != 0 {
		t.Fatalf("routes = %v, want none when neither consumer is enabled", routes)
	}
	reportsOnly := cartEventRoutes(true, false)
	if _, routed := reportsOnly[eventsDomain.TopicCartUpdated]; routed {
		t.Fatalf("cart.updated routed with the campaign producer disabled: %v", reportsOnly)
	}
	campaignOnly := cartEventRoutes(false, true)
	if _, routed := campaignOnly[eventsDomain.TopicCartCreated]; routed {
		t.Fatalf("carts.created routed with reports disabled: %v", campaignOnly)
	}
}

// TestEveryRoutedTopicHasAHandlerInThatConsumer is the check the previous audit
// got wrong: it confirmed a handler existed somewhere for each topic, not that
// it existed in the consumer the delivery is addressed to. That second pairing
// is the one that was broken.
func TestEveryRoutedTopicHasAHandlerInThatConsumer(t *testing.T) {
	handled := map[string][]string{
		eventsDomain.ConsumerReportsProjection: {
			eventsDomain.TopicOrderPaid, eventsDomain.TopicOrderRefunded,
			eventsDomain.TopicCartCreated, eventsDomain.TopicCheckoutStarted,
		},
		eventsDomain.ConsumerNotifications: {eventsDomain.TopicOrderPaid},
		abandonedApp.ConsumerCampaignProducer: {
			eventsDomain.TopicCartUpdated, eventsDomain.TopicCheckoutEmailCaptured,
		},
	}

	routed := make(map[string][]string)
	for topic, consumers := range orderWorkflowEventRoutes(true, true, true) {
		for _, consumer := range consumers {
			routed[consumer] = append(routed[consumer], topic)
		}
	}
	for topic, consumers := range cartEventRoutes(true, true) {
		for _, consumer := range consumers {
			routed[consumer] = append(routed[consumer], topic)
		}
	}

	for consumer, topics := range routed {
		known, described := handled[consumer]
		if !described {
			// Settlement and the rest are wired in their own builders; this
			// test covers the routes assembled here.
			continue
		}
		for _, topic := range topics {
			if !hasConsumer(known, topic) {
				t.Errorf("consumer %q is routed %q but registers no handler for it", consumer, topic)
			}
		}
	}
}

// TestNoPublisherFansOutAcrossTopics guards the shape rather than one call
// site: a publisher built with a consumer list sends every event it carries to
// every consumer on it, which is only safe where the caller publishes exactly
// one topic.
func TestNoPublisherFansOutAcrossTopics(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`NewPublisher\(([^)]*,[^)]*)\)`)
	var offenders []string
	if err := filepath.Walk(filepath.Join(root, "..", ".."), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range pattern.FindAllStringSubmatch(string(source), -1) {
			// NewPublisher(consumers...) expanding a slice is the variadic
			// forwarding inside the publisher package itself.
			if strings.Contains(match[1], "...") {
				continue
			}
			offenders = append(offenders, filepath.Base(path)+": "+match[0])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("publishers listing more than one consumer must route per topic instead:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func assertRoutedExactly(t *testing.T, routes map[string][]string, topic string, want ...string) {
	t.Helper()
	got := append([]string(nil), routes[topic]...)
	sort.Strings(got)
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	if strings.Join(got, ",") != strings.Join(expected, ",") {
		t.Fatalf("%s routed to %v, want %v", topic, got, expected)
	}
}
