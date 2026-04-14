package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/atlassian/jec/retryer"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
)

// deleteMessageEntry represents an entry in the batch delete payload.
type deleteMessageEntry struct {
	MessageId     string `json:"messageId"`
	MessageHandle string `json:"messageHandle"`
}

// deleteMessagesRequest is the request body for the batch delete API.
type deleteMessagesRequest struct {
	Messages []deleteMessageEntry `json:"messages"`
}

func deleteMessages(baseUrl, apiKey, channelId string, r *retryer.Retryer, entries []*trackedMessage) error {

	url := fmt.Sprintf("%s%s%s", baseUrl, messagesPath, channelId)

	deleteEntries := make([]deleteMessageEntry, 0, len(entries))
	for _, e := range entries {
		deleteEntries = append(deleteEntries, deleteMessageEntry{
			MessageId:     e.messageId,
			MessageHandle: e.messageHandle,
		})
	}
	requestBody := deleteMessagesRequest{Messages: deleteEntries}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal delete payload")
	}

	request, err := retryer.NewRequest(http.MethodDelete, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	request.Header.Add("Authorization", "GenieKey "+apiKey)
	request.Header.Add("X-JEC-Client-Info", UserAgentHeader)
	request.Header.Add("Content-Type", "application/json")

	response, err := r.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		body, _ := ioutil.ReadAll(response.Body)
		return errors.Errorf("Failed to delete messages from channel[%s], status: %s, message: %s", channelId, response.Status, body)
	}

	return nil
}
