package responses

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ds2api/internal/promptcompat"
)

func TestHandleResponsesStreamKeepsFunctionIdentityInCompletedResponse(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rec := httptest.NewRecorder()

	payload, _ := json.Marshal(map[string]any{
		"p": "response/content",
		"v": `<|DSML|tool_calls>
<|DSML|invoke name="lookup_secret">
<|DSML|parameter name="key"><![CDATA[vault-stream-9]]></|DSML|parameter>
</|DSML|invoke>
</|DSML|tool_calls>`,
	})
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("data: " + string(payload) + "\ndata: [DONE]\n")),
	}

	h.handleResponsesStream(
		rec,
		req,
		resp,
		"owner-a",
		"resp_identity",
		"deepseek-v4-flash",
		"prompt",
		0,
		false,
		false,
		[]string{"lookup_secret"},
		nil,
		promptcompat.DefaultToolChoicePolicy(),
		"",
	)

	body := rec.Body.String()
	addedEvents := extractSSEEventPayloads(body, "response.output_item.added")
	var streamedItem map[string]any
	for _, event := range addedEvents {
		item, _ := event["item"].(map[string]any)
		if item != nil && asString(item["type"]) == "function_call" {
			streamedItem = item
			break
		}
	}
	if streamedItem == nil {
		t.Fatalf("expected streamed function_call item, body=%s", body)
	}

	completed, ok := extractSSEEventPayload(body, "response.completed")
	if !ok {
		t.Fatalf("expected response.completed, body=%s", body)
	}
	response, _ := completed["response"].(map[string]any)
	output, _ := response["output"].([]any)
	var finalItem map[string]any
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item != nil && asString(item["type"]) == "function_call" {
			finalItem = item
			break
		}
	}
	if finalItem == nil {
		t.Fatalf("expected completed function_call item, payload=%#v", completed)
	}

	if got, want := asString(finalItem["id"]), asString(streamedItem["id"]); got == "" || got != want {
		t.Fatalf("completed item id = %q, want streamed id %q; body=%s", got, want, body)
	}
	if got, want := asString(finalItem["call_id"]), asString(streamedItem["call_id"]); got == "" || got != want {
		t.Fatalf("completed call_id = %q, want streamed call_id %q; body=%s", got, want, body)
	}
}
