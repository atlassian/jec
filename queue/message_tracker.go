package queue

// trackedMessage represents a message being tracked by the tracker.
type trackedMessage struct {
	messageId     string
	messageHandle string
	processed     bool
}

// messageTracker tracks fetched messages and their submission status.
// It is only accessed from within poll(), which runs in a single goroutine,
// so no mutex/thread-safety is needed.
type messageTracker struct {
	messages map[string]*trackedMessage
}

// newMessageTracker creates a new message tracker.
func newMessageTracker() *messageTracker {
	return &messageTracker{
		messages: make(map[string]*trackedMessage),
	}
}

// putIfAbsent adds a message to the tracker if not already tracked.
// If the message is already tracked, it does nothing.
func (mr *messageTracker) putIfAbsent(messageId, messageHandle string) {
	if _, exists := mr.messages[messageId]; !exists {
		mr.messages[messageId] = &trackedMessage{
			messageId:     messageId,
			messageHandle: messageHandle,
			processed:     false,
		}
	}
}

// IsProcessed checks if a message has already been processed (for dedup).
func (mr *messageTracker) IsProcessed(messageId string) bool {
	if msg, exists := mr.messages[messageId]; exists {
		return msg.processed
	}
	return false
}

// MarkProcessed marks a message as processed.
func (mr *messageTracker) MarkProcessed(messageId string) {
	if msg, exists := mr.messages[messageId]; exists {
		msg.processed = true
	}
}

// AllProcessed returns true if all tracked messages have been processed.
func (mr *messageTracker) AllProcessed() bool {
	if len(mr.messages) == 0 {
		return false
	}
	for _, msg := range mr.messages {
		if !msg.processed {
			return false
		}
	}
	return true
}

// ProcessedEntries returns a list of processed tracked messages.
func (mr *messageTracker) ProcessedEntries() []*trackedMessage {
	entries := make([]*trackedMessage, 0)
	for _, msg := range mr.messages {
		if msg.processed {
			entries = append(entries, msg)
		}
	}
	return entries
}

// Reset clears all tracked messages.
func (mr *messageTracker) Reset() {
	mr.messages = make(map[string]*trackedMessage)
}
