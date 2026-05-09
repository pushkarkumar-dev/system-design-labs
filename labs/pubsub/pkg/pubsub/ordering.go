package pubsub

// ── Ordering keys (v2) ────────────────────────────────────────────────────────
//
// Messages with the same OrderingKey are delivered to a subscriber in publish
// order. Messages with different (or empty) ordering keys may be delivered
// concurrently or in any order relative to each other.
//
// Implementation: each subscription maintains a map[orderingKey]chan Message.
// Each key-channel is drained by its own goroutine, so messages with key "A"
// never block messages with key "B" — only messages within the same key are
// serialised.
//
// Memory management: key channels are created lazily on first message with that
// key and live for the lifetime of the subscription. In production you would
// add a reaping strategy for inactive keys.

// deliverOrdered routes msg to the per-key goroutine for this subscription.
// If the key is new, a new channel and goroutine are created.
// Messages with an empty OrderingKey bypass per-key serialisation and are
// delivered directly to the main channel.
func deliverOrdered(sub *Subscription, msg Message) {
	if msg.OrderingKey == "" {
		// No ordering guarantee needed — fan into the main channel.
		deliverAsync(sub, msg)
		return
	}

	sub.orderingMu.Lock()
	keyCh, ok := sub.orderingKeys[msg.OrderingKey]
	if !ok {
		// First message with this ordering key: create a dedicated channel.
		keyCh = make(chan Message, 1000)
		sub.orderingKeys[msg.OrderingKey] = keyCh
		sub.wg.Add(1)
		go sub.keyLoop(keyCh)
	}
	sub.orderingMu.Unlock()

	// Deliver to the per-key channel (non-blocking, Drop policy).
	select {
	case keyCh <- msg:
	default:
		// Key channel full — apply backpressure policy.
		if sub.backpressure == Block {
			keyCh <- msg
		} else {
			// Drop policy.
			// We don't increment the stats.Dropped counter here because the
			// drop happened in the ordering layer, not the main channel.
		}
	}
}

// keyLoop is the per-ordering-key goroutine. It reads from keyCh and forwards
// each message to the main subscription channel in strict order.
// This ensures that all messages with the same ordering key arrive at the
// subscriber's ch in the exact order they were published.
func (sub *Subscription) keyLoop(keyCh <-chan Message) {
	defer sub.wg.Done()
	for {
		select {
		case <-sub.stopCh:
			return
		case msg, ok := <-keyCh:
			if !ok {
				return
			}
			// Forward to the main delivery channel so the subscriber reads a
			// single unified channel regardless of ordering key.
			select {
			case sub.ch <- msg:
			case <-sub.stopCh:
				return
			}
		}
	}
}
