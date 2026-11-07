package thinking

import (
	"encoding/json"
	"testing"
)

// TestLegacyInboundEnvelopeRejected 旧信封一律无效：data URI、本机只读路径、裸网络地址都不再放行
func TestLegacyInboundEnvelopeRejected(t *testing.T) {
	cases := []struct {
		name string
		item ContentItem
	}{
		{"data_uri_image", ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: "data:image/png;base64,iVBORw0KGgo="}}},
		{"local_read_path", ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: "/file/read/multimedia/abc.png"}}},
		{"remote_image", ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: "https://example.com/a.png"}}},
		{"video_url", ContentItem{Type: ContentVideoURL}},
		{"audio_url", ContentItem{Type: ContentAudioURL}},
		{"input_audio", ContentItem{Type: ContentInputAudio}},
	}
	for _, c := range cases {
		if reason, legacy := legacyInboundEnvelope(c.item); !legacy {
			t.Errorf("%s 应判定为旧信封，实际被放行（reason=%q）", c.name, reason)
		}
	}
}

// TestLegacyInboundEnvelopeAllowsServerArtifact 服务端清洗产物（带资源句柄）不受拦截
func TestLegacyInboundEnvelopeAllowsServerArtifact(t *testing.T) {
	item := ContentItem{
		Type:           ContentImageURL,
		ImageURL:       &URLPayload{URL: "data:image/png;base64,iVBORw0KGgo="},
		ResourceHandle: "@I.1",
	}
	if reason, legacy := legacyInboundEnvelope(item); legacy {
		t.Fatalf("服务端清洗产物被误判为旧信封：%s", reason)
	}
}

// TestSenderCannotForgeResourceHandle resource_handle 不参与 JSON 解码，发送侧无法伪装成服务端产物
func TestSenderCannotForgeResourceHandle(t *testing.T) {
	raw := []byte(`{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="},"resource_handle":"@I.1"}`)
	var item ContentItem
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatalf("解析内容项失败: %v", err)
	}
	if item.ResourceHandle != "" {
		t.Fatalf("resource_handle 不应从 JSON 还原，实际为 %q", item.ResourceHandle)
	}
	if _, legacy := legacyInboundEnvelope(item); !legacy {
		t.Fatal("伪造 resource_handle 的 image_url 应判定为旧信封")
	}
}
