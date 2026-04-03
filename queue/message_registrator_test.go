package queue

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewMessageRegistrator(t *testing.T) {
	mr := newMessageRegistrator()
	assert.NotNil(t, mr)
	assert.NotNil(t, mr.messages)
	assert.Equal(t, 0, len(mr.messages))
}

func TestAdd(t *testing.T) {
	mr := newMessageRegistrator()

	mr.Add("msg1", "handle1")
	assert.Equal(t, 1, len(mr.messages))
	assert.NotNil(t, mr.messages["msg1"])
	assert.Equal(t, "msg1", mr.messages["msg1"].messageId)
	assert.Equal(t, "handle1", mr.messages["msg1"].messageHandle)
	assert.False(t, mr.messages["msg1"].processed)
}

func TestAddDedup(t *testing.T) {
	mr := newMessageRegistrator()

	mr.Add("msg1", "handle1")
	mr.Add("msg1", "handle2")

	// Second add should not overwrite the first
	assert.Equal(t, 1, len(mr.messages))
	assert.Equal(t, "handle1", mr.messages["msg1"].messageHandle)
}

func TestAddMultiple(t *testing.T) {
	mr := newMessageRegistrator()

	mr.Add("msg1", "handle1")
	mr.Add("msg2", "handle2")
	mr.Add("msg3", "handle3")

	assert.Equal(t, 3, len(mr.messages))
	assert.Equal(t, "msg1", mr.messages["msg1"].messageId)
	assert.Equal(t, "msg2", mr.messages["msg2"].messageId)
	assert.Equal(t, "msg3", mr.messages["msg3"].messageId)
}

func TestIsProcessed(t *testing.T) {
	mr := newMessageRegistrator()

	mr.Add("msg1", "handle1")

	// Initially not processed
	assert.False(t, mr.IsProcessed("msg1"))

	// Mark as processed
	mr.MarkProcessed("msg1")
	assert.True(t, mr.IsProcessed("msg1"))
}

func TestIsProcessedNonExistent(t *testing.T) {
	mr := newMessageRegistrator()

	// Non-existent message should return false
	assert.False(t, mr.IsProcessed("msg1"))
}

func TestMarkProcessed(t *testing.T) {
	mr := newMessageRegistrator()

	mr.Add("msg1", "handle1")
	mr.Add("msg2", "handle2")

	mr.MarkProcessed("msg1")

	assert.True(t, mr.IsProcessed("msg1"))
	assert.False(t, mr.IsProcessed("msg2"))
}

func TestMarkProcessedNonExistent(t *testing.T) {
	mr := newMessageRegistrator()

	// Marking a non-existent message should not panic
	mr.MarkProcessed("msg1")
	assert.Equal(t, 0, len(mr.messages))
}

func TestAllProcessed(t *testing.T) {
	mr := newMessageRegistrator()

	// Empty registrator should return false
	assert.False(t, mr.AllProcessed())

	// Add one message
	mr.Add("msg1", "handle1")
	assert.False(t, mr.AllProcessed())

	// Mark it as processed
	mr.MarkProcessed("msg1")
	assert.True(t, mr.AllProcessed())

	// Add another message
	mr.Add("msg2", "handle2")
	assert.False(t, mr.AllProcessed())

	// Mark it as processed
	mr.MarkProcessed("msg2")
	assert.True(t, mr.AllProcessed())
}

func TestProcessedEntries(t *testing.T) {
	mr := newMessageRegistrator()

	mr.Add("msg1", "handle1")
	mr.Add("msg2", "handle2")
	mr.Add("msg3", "handle3")

	// No messages processed yet
	entries := mr.ProcessedEntries()
	assert.Equal(t, 0, len(entries))

	// Mark some as processed
	mr.MarkProcessed("msg1")
	mr.MarkProcessed("msg3")

	entries = mr.ProcessedEntries()
	assert.Equal(t, 2, len(entries))

	// Check that entries contain the correct data
	found := make(map[string]string)
	for _, entry := range entries {
		found[entry.messageId] = entry.messageHandle
	}
	assert.Equal(t, "handle1", found["msg1"])
	assert.Equal(t, "handle3", found["msg3"])
	assert.NotContains(t, found, "msg2")
}

func TestProcessedEntriesEmpty(t *testing.T) {
	mr := newMessageRegistrator()

	entries := mr.ProcessedEntries()
	assert.Equal(t, 0, len(entries))
	assert.NotNil(t, entries) // Should be empty slice, not nil
}

func TestReset(t *testing.T) {
	mr := newMessageRegistrator()

	mr.Add("msg1", "handle1")
	mr.Add("msg2", "handle2")
	mr.MarkProcessed("msg1")

	assert.Equal(t, 2, len(mr.messages))
	assert.True(t, mr.IsProcessed("msg1"))

	mr.Reset()

	assert.Equal(t, 0, len(mr.messages))
	assert.False(t, mr.IsProcessed("msg1"))
	assert.False(t, mr.AllProcessed())
}

func TestResetEmpty(t *testing.T) {
	mr := newMessageRegistrator()

	// Reset on empty registrator should not panic
	mr.Reset()
	assert.Equal(t, 0, len(mr.messages))
}

func TestCompleteWorkflow(t *testing.T) {
	mr := newMessageRegistrator()

	// Fetch and add messages
	mr.Add("msg1", "handle1")
	mr.Add("msg2", "handle2")
	mr.Add("msg3", "handle3")

	// Dedup check - msg1 should not be added again
	assert.False(t, mr.IsProcessed("msg1"))
	mr.Add("msg1", "newhandle1") // Should not overwrite

	// Submit and mark processed
	mr.MarkProcessed("msg1")
	mr.MarkProcessed("msg2")
	mr.MarkProcessed("msg3")

	// Check all processed
	assert.True(t, mr.AllProcessed())

	// Get entries for delete
	entries := mr.ProcessedEntries()
	assert.Equal(t, 3, len(entries))

	// Reset after successful delete
	mr.Reset()
	assert.Equal(t, 0, len(mr.messages))
	assert.False(t, mr.AllProcessed())
}
