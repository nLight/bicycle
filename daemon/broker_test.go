package daemon

import (
	"context"
	"sync"
	"testing"
	"time"

	"bicycle/plugin"
)

func TestBroker_NewBroker(t *testing.T) {
	b := NewBroker()
	if b == nil {
		t.Fatal("NewBroker returned nil")
	}
	if b.closed {
		t.Error("New broker should not be closed")
	}
	if b.SubscriberCount() != 0 {
		t.Errorf("New broker should have 0 subscribers, got %d", b.SubscriberCount())
	}
}

func TestBroker_Subscribe(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch := b.Subscribe("test-sub", 10, "topic1", "topic2")
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
	if b.SubscriberCount() != 1 {
		t.Errorf("Expected 1 subscriber, got %d", b.SubscriberCount())
	}
}

func TestBroker_SubscribeReplace(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch1 := b.Subscribe("test-sub", 10, "topic1")
	ch2 := b.Subscribe("test-sub", 10, "topic2") // Same ID, should replace

	if b.SubscriberCount() != 1 {
		t.Errorf("Expected 1 subscriber after replace, got %d", b.SubscriberCount())
	}

	// Old channel should be closed
	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("Old channel should be closed")
		}
	default:
		t.Error("Old channel should be closed and readable")
	}

	_ = ch2 // New channel should be valid
}

func TestBroker_Publish(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch := b.Subscribe("test-sub", 10, "test-topic")

	ctx := context.Background()
	msg := plugin.Message{
		Topic:   "test-topic",
		Payload: "test payload",
		Source:  "test",
	}

	err := b.Publish(ctx, msg)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case received := <-ch:
		if received.Topic != msg.Topic {
			t.Errorf("Expected topic %s, got %s", msg.Topic, received.Topic)
		}
		if received.Payload != msg.Payload {
			t.Errorf("Expected payload %v, got %v", msg.Payload, received.Payload)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for message")
	}
}

func TestBroker_PublishNoSubscribers(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ctx := context.Background()
	msg := plugin.Message{
		Topic:   "unsubscribed-topic",
		Payload: "test",
		Source:  "test",
	}

	// Should not error when no subscribers
	err := b.Publish(ctx, msg)
	if err != nil {
		t.Errorf("Publish with no subscribers should not error: %v", err)
	}
}

func TestBroker_PublishWildcard(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch := b.Subscribe("wildcard-sub", 10, "*")

	ctx := context.Background()
	msg := plugin.Message{
		Topic:   "any-topic",
		Payload: "test",
		Source:  "test",
	}

	err := b.Publish(ctx, msg)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case received := <-ch:
		if received.Topic != msg.Topic {
			t.Errorf("Expected topic %s, got %s", msg.Topic, received.Topic)
		}
	case <-time.After(time.Second):
		t.Error("Wildcard subscriber should receive message")
	}
}

func TestBroker_PublishTopicFiltering(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch1 := b.Subscribe("sub1", 10, "topic-a")
	ch2 := b.Subscribe("sub2", 10, "topic-b")

	ctx := context.Background()
	msg := plugin.Message{
		Topic:   "topic-a",
		Payload: "test",
		Source:  "test",
	}

	err := b.Publish(ctx, msg)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// ch1 should receive (subscribed to topic-a)
	select {
	case <-ch1:
		// Good
	case <-time.After(time.Second):
		t.Error("Subscriber to topic-a should receive message")
	}

	// ch2 should not receive (subscribed to topic-b)
	select {
	case <-ch2:
		t.Error("Subscriber to topic-b should not receive topic-a message")
	case <-time.After(100 * time.Millisecond):
		// Good - no message
	}
}

func TestBroker_Unsubscribe(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch := b.Subscribe("test-sub", 10, "topic")
	if b.SubscriberCount() != 1 {
		t.Fatal("Expected 1 subscriber")
	}

	b.Unsubscribe("test-sub")

	if b.SubscriberCount() != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", b.SubscriberCount())
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after unsubscribe")
		}
	default:
		t.Error("Channel should be closed and readable")
	}
}

func TestBroker_Close(t *testing.T) {
	b := NewBroker()

	ch1 := b.Subscribe("sub1", 10, "topic")
	ch2 := b.Subscribe("sub2", 10, "topic")

	b.Close()

	// All channels should be closed
	for i, ch := range []<-chan plugin.Message{ch1, ch2} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("Channel %d should be closed", i)
			}
		default:
			t.Errorf("Channel %d should be closed and readable", i)
		}
	}

	// Publish should fail on closed broker
	ctx := context.Background()
	err := b.Publish(ctx, plugin.Message{Topic: "test"})
	if err == nil {
		t.Error("Publish on closed broker should error")
	}
}

func TestBroker_ConcurrentPublish(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	ch := b.Subscribe("test-sub", 100, "topic")
	ctx := context.Background()

	var wg sync.WaitGroup
	numMessages := 50

	// Publish concurrently
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			b.Publish(ctx, plugin.Message{
				Topic:   "topic",
				Payload: n,
				Source:  "test",
			})
		}(i)
	}

	wg.Wait()

	// Count received messages
	received := 0
	timeout := time.After(time.Second)
loop:
	for {
		select {
		case <-ch:
			received++
			if received == numMessages {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if received != numMessages {
		t.Errorf("Expected %d messages, received %d", numMessages, received)
	}
}

func TestBroker_SetPublishTimeout(t *testing.T) {
	b := NewBroker()
	defer b.Close()

	b.SetPublishTimeout(100 * time.Millisecond)

	// Subscribe with buffer size 1
	ch := b.Subscribe("slow-sub", 1, "topic")

	ctx := context.Background()

	// Fill the buffer
	b.Publish(ctx, plugin.Message{Topic: "topic", Payload: "1"})

	// This should timeout (buffer full, no one reading)
	start := time.Now()
	err := b.Publish(ctx, plugin.Message{Topic: "topic", Payload: "2"})
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error for slow consumer")
	}

	if elapsed < 90*time.Millisecond {
		t.Errorf("Expected timeout after ~100ms, got %v", elapsed)
	}

	_ = ch // Avoid unused warning
}
