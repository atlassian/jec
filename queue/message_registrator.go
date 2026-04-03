package queue

// trackedMessage represents a message being tracked by the registrator.
type trackedMessage struct {
	messageId     string
	messageHandle string
	processed     bool
}

// messageRegistrator tracks fetched messages and their submission status.
// It is only accessed from within poll(), which runs in a single goroutine,
// so no mutex/thread-safety is needed.
type messageRegistrator struct {
	messages map[string]*trackedMessage
}

// newMessageRegistrator creates a new message registrator.
func newMessageRegistrator() *messageRegistrator {
	return &messageRegistrator{
		messages: make(map[string]*trackedMessage),
	}
}

// Add adds a message to the registrator if not already tracked.
// If the message is already tracked, it does nothing.
func (mr *messageRegistrator) Add(messageId, messageHandle string) {
	if _, exists := mr.messages[messageId]; !exists {
		mr.messages[messageId] = &trackedMessage{
			messageId:     messageId,
			messageHandle: messageHandle,
			processed:     false,
		}
	}
}

// IsProcessed checks if a message has already been processed (for dedup).
func (mr *messageRegistrator) IsProcessed(messageId string) bool {
	if msg, exists := mr.messages[messageId]; exists {
		return msg.processed
	}
	return false
}

// MarkProcessed marks a message as processed.
func (mr *messageRegistrator) MarkProcessed(messageId string) {
	if msg, exists := mr.messages[messageId]; exists {
		msg.processed = true
	}
}

// AllProcessed returns true if all tracked messages have been processed.
func (mr *messageRegistrator) AllProcessed() bool {
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
func (mr *messageRegistrator) ProcessedEntries() []*trackedMessage {
	entries := make([]*trackedMessage, 0)
	for _, msg := range mr.messages {
		if msg.processed {
			entries = append(entries, msg)
		}
	}
	return entries
}

// Reset clears all tracked messages.
func (mr *messageRegistrator) Reset() {
	mr.messages = make(map[string]*trackedMessage)
}
