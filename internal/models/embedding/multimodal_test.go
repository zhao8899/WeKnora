package embedding

import (
	"testing"
)

// Compile-time checks that Volcengine and Aliyun embedders implement MultimodalEmbedder.
var _ MultimodalEmbedder = (*VolcengineEmbedder)(nil)
var _ MultimodalEmbedder = (*AliyunEmbedder)(nil)

func TestMultimodalEmbedderInterface(t *testing.T) {
	// Verify that MultimodalEmbedder is a superset of Embedder
	var me MultimodalEmbedder
	_ = Embedder(me) // must be assignable

	t.Log("VolcengineEmbedder and AliyunEmbedder satisfy MultimodalEmbedder")
}
