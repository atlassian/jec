package queue

// JECMessage represents a message fetched from the JEC API.
type JECMessage struct {
	MessageId string `json:"messageId"`
	Body      string `json:"body"`
	ChannelId string `json:"channelId"`
}
