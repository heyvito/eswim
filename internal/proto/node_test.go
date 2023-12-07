package proto

import (
	"github.com/heyvito/eswim/internal/containers"
	"github.com/stretchr/testify/assert"
	"testing"
)

var nodeTestNodesToTest = []containers.Triple[string, Node, []byte]{
	containers.Tri("ipv4 Healthy", ipv4Healthy, ipv4HealthyBytes),
	containers.Tri("ipv4 Suspect", ipv4Suspect, ipv4SuspectBytes),
	containers.Tri("ipv6 Healthy", ipv6Healthy, ipv6HealthyBytes),
	containers.Tri("ipv6 Suspect", ipv6Suspect, ipv6SuspectBytes),
}

func TestNode_Encode(t *testing.T) {
	t.Parallel()
	for _, v := range nodeTestNodesToTest {
		name, node, buffer := v.Decompose()
		t.Run(name, func(t *testing.T) {
			data := encodeEncoder(node)
			assert.Equal(t, buffer, data)
		})
	}
}

func TestNode_Decode(t *testing.T) {
	t.Parallel()
	for _, v := range nodeTestNodesToTest {
		name, node, buffer := v.Decompose()
		t.Run(name, func(t *testing.T) {
			decoded := decodeInto(t, nodeDecoder, buffer)
			assert.Equal(t, &node, decoded)
		})
	}
}
