package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"bicycle/plugin"

	"golang.org/x/sync/errgroup"
)

// Subscription represents a subscriber's subscription
type Subscription struct {
	id      string
	ch      chan plugin.Message
	topics  []string
	bufSize int
}

// Broker implements a topic-based pub/sub message broker
type Broker struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription
	closed        bool
	publishTimeout time.Duration
}

// NewBroker creates a new message broker
func NewBroker() *Broker {
	return &Broker{
		subscriptions: make(map[string]*Subscription),
		closed:        false,
		publishTimeout: 5 * time.Second, // Default timeout for slow consumers
	}
}

// Subscribe creates a new subscription for the given topics
// Returns a channel that will receive matching messages
func (b *Broker) Subscribe(id string, bufSize int, topics ...string) <-chan plugin.Message {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		log.Printf("[Broker] Warning: Subscribe called on closed broker for %s", id)
		ch := make(chan plugin.Message)
		close(ch)
		return ch
	}

	// If subscription already exists, close old channel and replace
	if old, exists := b.subscriptions[id]; exists {
		log.Printf("[Broker] Replacing existing subscription for %s", id)
		close(old.ch)
	}

	sub := &Subscription{
		id:      id,
		ch:      make(chan plugin.Message, bufSize),
		topics:  topics,
		bufSize: bufSize,
	}

	b.subscriptions[id] = sub
	log.Printf("[Broker] %s subscribed to topics: %v (buffer: %d)", id, topics, bufSize)

	return sub.ch
}

// Publish broadcasts a message to all interested subscribers
// Uses fan-out pattern with concurrent delivery and timeout handling
func (b *Broker) Publish(ctx context.Context, msg plugin.Message) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("broker is closed")
	}

	// Find matching subscriptions
	var targets []*Subscription
	for _, sub := range b.subscriptions {
		if sub.wantsTopic(msg.Topic) {
			targets = append(targets, sub)
		}
	}

	if len(targets) == 0 {
		// No subscribers for this topic - not an error
		log.Printf("[Broker] No subscribers for topic: %s", msg.Topic)
		return nil
	}

	// Fan-out: publish to all subscribers concurrently
	g, gctx := errgroup.WithContext(ctx)

	for _, sub := range targets {
		sub := sub // Capture loop variable
		g.Go(func() error {
			return b.publishToSubscriber(gctx, sub, msg)
		})
	}

	// Wait for all publishes to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	log.Printf("[Broker] Published message (topic: %s, source: %s) to %d subscriber(s)", msg.Topic, msg.Source, len(targets))
	return nil
}

// publishToSubscriber sends a message to a single subscriber with timeout
func (b *Broker) publishToSubscriber(ctx context.Context, sub *Subscription, msg plugin.Message) error {
	select {
	case sub.ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(b.publishTimeout):
		// Slow consumer - this is a policy decision
		// We could: 1) drop the message, 2) return error, 3) block forever
		// Here we return an error to alert that the subscriber is slow
		return fmt.Errorf("timeout publishing to %s (slow consumer)", sub.id)
	}
}

// Unsubscribe removes a subscription and closes its channel
func (b *Broker) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sub, ok := b.subscriptions[id]; ok {
		close(sub.ch)
		delete(b.subscriptions, id)
		log.Printf("[Broker] %s unsubscribed", id)
	}
}

// Close shuts down the broker and closes all subscription channels
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true

	// Close all subscription channels
	for id, sub := range b.subscriptions {
		close(sub.ch)
		log.Printf("[Broker] Closed subscription: %s", id)
	}

	// Clear subscriptions
	b.subscriptions = make(map[string]*Subscription)

	log.Println("[Broker] Broker closed")
}

// SubscriberCount returns the current number of subscribers
func (b *Broker) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscriptions)
}

// SetPublishTimeout sets the timeout for publishing to slow consumers
func (b *Broker) SetPublishTimeout(timeout time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.publishTimeout = timeout
}

// wantsTopic checks if a subscription is interested in a topic
func (s *Subscription) wantsTopic(topic string) bool {
	// Empty topics list means subscribe to all
	if len(s.topics) == 0 {
		return true
	}

	// Check for exact match or wildcard
	for _, t := range s.topics {
		if t == topic || t == "*" {
			return true
		}
		// Could add pattern matching here (e.g., "command.*")
	}

	return false
}
