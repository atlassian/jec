package runbook

import (
	"bytes"
	"encoding/json"
	"github.com/atlassian/jec/retryer"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"strconv"
)

const resultPath = "/jsm/ops/jec/v1/callback"

var SendResultToJsmFunc = SendResultToJsm

var client = &retryer.Retryer{}

type ActionResultPayload struct {
	RequestId       string `json:"requestId,omitempty"`
	IsSuccessful    bool   `json:"isSuccessful,omitempty"`
	EntityId        string `json:"entityId,omitempty"`
	EntityType      string `json:"entityType,omitempty"`
	Action          string `json:"action,omitempty"`
	ActionType      string `json:"actionType,omitempty"`
	FailureMessage  string `json:"failureMessage,omitempty"`
	CallbackContext string `json:"callbackContext,omitempty"`
	*HttpResponse
}

type HttpResponse struct {
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	StatusCode int               `json:"statusCode"`
}

func SendResultToJsm(resultPayload *ActionResultPayload, apiKey, baseUrl string) error {

	body, err := json.Marshal(resultPayload)
	if err != nil {
		return errors.Errorf("Cannot marshall payload: %s", err)
	}

	resultUrl := baseUrl + resultPath

	request, err := retryer.NewRequest(http.MethodPost, resultUrl, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Add("Authorization", "GenieKey "+apiKey)
	request.Header.Add("Content-Type", "application/json; charset=UTF-8")

	response, err := client.Do(request)
	if err != nil {
		return err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {

		errorMessage := "Unexpected response status: " + strconv.Itoa(response.StatusCode)

		body, err := ioutil.ReadAll(response.Body)
		if err == nil {
			return errors.Errorf("%s, error message: %s", errorMessage, string(body))
		} else {
			return errors.Errorf("%s, also could not read response body: %s", errorMessage, err)
		}
	}

	return nil
}
