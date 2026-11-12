package app

import "testing"

// TestResolveCreationDomain 资源库创作按领域分流；旧客户端不带 domain 时按描述推断。
func TestResolveCreationDomain(t *testing.T) {
	cases := []struct {
		domain      string
		instruction string
		want        string
	}{
		{domain: "story", instruction: "随便写", want: creationDomainStory},
		{domain: "edit", instruction: "写个故事", want: creationDomainEdit},
		{domain: "", instruction: "续写《雨夜旧书店》下一章", want: creationDomainStory},
		{domain: "", instruction: "把 documents/说明.md 的错别字改掉", want: creationDomainEdit},
	}
	for _, c := range cases {
		if got := resolveCreationDomain(c.domain, c.instruction); got != c.want {
			t.Fatalf("resolveCreationDomain(%q, %q) = %q，期望 %q", c.domain, c.instruction, got, c.want)
		}
	}
	if got := creationDomainStage(creationDomainStory); got != "story_writer" {
		t.Fatalf("故事领域的阶段名应为 story_writer，实际 %q", got)
	}
	if got := creationDomainStage(creationDomainEdit); got != "file_operations" {
		t.Fatalf("编辑领域的阶段名应为 file_operations，实际 %q", got)
	}
}
