package outbox

import (
	"encoding/json"
	"testing"
)

func TestAMQPHeaderValuePreservesIntegralJSONNumbers(t *testing.T) {
	value := amqpHeaderValue(json.Number("3"))
	integer, ok := value.(int64)
	if !ok || integer != 3 {
		t.Fatalf("amqpHeaderValue() = %#v", value)
	}
}
