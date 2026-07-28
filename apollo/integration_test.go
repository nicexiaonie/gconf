package apollo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestIntegrationApolloFormats 连接真实 Apollo，验证 config.json（源 JSON）
// 可转格式为 json/yaml/xml 落盘，且落盘内容可按目标格式解析回来。
func TestIntegrationApolloFormats(t *testing.T) {
	cases := []struct {
		name   string
		format Format
	}{
		{"json", FormatJSON},
		{"yaml", FormatYAML},
		{"xml", FormatXML},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := Config{
				AppID:      "Conduit-Service",
				Cluster:    "default",
				MetaServer: "http://159.75.184.171:32369",
				CacheDir:   dir,
				Namespaces: []NamespaceConfig{
					{Name: "config.json", Required: true, Format: c.format},
				},
			}
			syncer, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := syncer.Start(context.Background()); err != nil {
				t.Fatalf("start: %v", err)
			}
			defer syncer.Close()

			st := syncer.Status()
			t.Logf("status: source=%s hash=%s", st.Namespaces[0].Source, st.Namespaces[0].Hash)

			path := filepath.Join(dir, "config.json", "config."+string(c.format))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read published file %s: %v", path, err)
			}

			// 验证可按目标格式解析回来
			values, err := decode(data, c.format)
			if err != nil {
				t.Fatalf("decode as %s: %v", c.format, err)
			}
			if len(values) == 0 {
				t.Fatalf("decoded map empty")
			}

			content := string(data)
			preview := content
			if len(preview) > 300 {
				preview = preview[:300] + fmt.Sprintf("\n...(%d bytes total)", len(content))
			}
			t.Logf("[%s] published %d bytes, top-level keys=%d:\n%s",
				c.format, len(data), len(values), preview)
		})
	}
}
