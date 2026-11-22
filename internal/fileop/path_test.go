package fileop

import (
	"testing"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

const root = "/var/lib/kore/file"

// TestValidatePathAccept 合法相对路径应归一化到根目录内。
func TestValidatePathAccept(t *testing.T) {
	cases := map[string]string{
		"docs":       "/var/lib/kore/file/docs",
		"docs/a.txt": "/var/lib/kore/file/docs/a.txt",
		".":          root,
		"a//b":       "/var/lib/kore/file/a/b",
	}
	for raw, want := range cases {
		got, perr := ValidatePath(root, raw)
		if perr != nil {
			t.Fatalf("ValidatePath(%q) 意外拒绝: %v", raw, perr)
		}
		if got != want {
			t.Fatalf("ValidatePath(%q)=%q，期望 %q", raw, got, want)
		}
	}
}

// TestValidatePathReject 各类穿越与非法路径必须返回 E_INVALID。
func TestValidatePathReject(t *testing.T) {
	bad := []string{
		"", "/etc/passwd", "../etc", "a/../../b", "docs/../..",
		"a/b/../c.txt", "x/../y",
		`docs\a.txt`, "docs/\x00a", "/../escape", "../../../../etc/shadow",
	}
	for _, raw := range bad {
		got, perr := ValidatePath(root, raw)
		if perr == nil {
			t.Fatalf("ValidatePath(%q) 未拒绝（返回 %q）", raw, got)
		}
		if perr.Code != pluginapi.E_INVALID {
			t.Fatalf("ValidatePath(%q) 错误码=%q，期望 E_INVALID", raw, perr.Code)
		}
		if got != "" {
			t.Fatalf("ValidatePath(%q) 拒绝时不应返回路径: %q", raw, got)
		}
	}
}

// TestValidatePathRootPrefix 校验返回值始终以根目录为前缀或等于根目录。
func TestValidatePathRootPrefix(t *testing.T) {
	for _, raw := range []string{"docs", "a/b/c", "docs/a.txt"} {
		got, perr := ValidatePath(root, raw)
		if perr != nil {
			t.Fatalf("ValidatePath(%q): %v", raw, perr)
		}
		if !hasRootPrefix(got) {
			t.Fatalf("ValidatePath(%q)=%q 越出根目录 %q", raw, got, root)
		}
	}
}

func hasRootPrefix(p string) bool {
	return p == root || len(p) > len(root) && p[:len(root)+1] == root+"/"
}
