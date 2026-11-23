package fileop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// methodActionPattern 与内核 registry 的方法级能力键格式一致（validate.go 的 methodActionPattern）。
const methodActionPattern = `^[a-z][a-zA-Z0-9]*(\.[a-z][a-zA-Z0-9]*)*$`

// privilegedCapabilities 是内核唯一认识的 6 项特权能力（PLUGIN-PROTOCOL §3.1.2）。
var privilegedCapabilities = map[string]bool{
	"system:read": true, "system:admin": true,
	"net:read": true, "net:write": true,
	"proc:read": true, "proc:write": true,
}

// infoManifest 只声明本测试关心的字段；其余字段由内核校验，这里不重复建模。
type infoManifest struct {
	ID                 string            `json:"id"`
	Version            string            `json:"version"`
	Capabilities       []string          `json:"capabilities"`
	MethodCapabilities map[string]string `json:"method_capabilities"`
}

// TestMethodCapabilitiesDeclared 方法级能力必须覆盖全部 19 个业务方法，且只读/写分类正确。
// 这条测试是 info.json 与 call.go 分发表之间的防漂移门禁（§3.5 #12）。
func TestMethodCapabilitiesDeclared(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "info.json"))
	if err != nil {
		t.Fatalf("读取 info.json 失败: %v", err)
	}
	var manifest infoManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("解析 info.json 失败: %v", err)
	}
	if manifest.ID != "file" {
		t.Fatalf("info.json.id=%q，期望 file", manifest.ID)
	}
	readOnly := []string{"list", "read", "stat", "download", "search", "trash.list"}
	writeOps := []string{
		"write", "delete", "mkdir", "chmod", "move", "rename", "copy",
		"upload", "compress", "extract", "trash.restore", "trash.purge", "batch",
	}
	if want := len(readOnly) + len(writeOps); len(manifest.MethodCapabilities) != want {
		t.Fatalf("method_capabilities 条数=%d，期望 %d", len(manifest.MethodCapabilities), want)
	}
	for _, verb := range readOnly {
		assertMethodCapability(t, manifest, verb, "system:read")
	}
	for _, verb := range writeOps {
		assertMethodCapability(t, manifest, verb, "system:admin")
	}
	// 镜像内核校验（registry/validate.go:157-194）：键格式、值域、条数上限。
	if len(manifest.MethodCapabilities) > 64 {
		t.Fatalf("method_capabilities 条数=%d 超过内核上限 64", len(manifest.MethodCapabilities))
	}
	pattern := regexp.MustCompile(methodActionPattern)
	for key, value := range manifest.MethodCapabilities {
		action, ok := strings.CutPrefix(key, "file.")
		if !ok || action == "" || !pattern.MatchString(action) {
			t.Fatalf("方法级能力键 %q 不符合内核格式要求", key)
		}
		if !privilegedCapabilities[value] {
			t.Fatalf("%s 的能力 %q 不是 6 项特权能力之一", key, value)
		}
	}
	if len(manifest.Capabilities) == 0 {
		t.Fatal("插件级 capabilities 不能为空")
	}
	if _, ok := businessVerb("file.exec"); ok {
		t.Fatal("businessVerb 不应接受未实现的方法 file.exec")
	}
}

// assertMethodCapability 断言单条声明存在、取值正确，且该动词确实能被分发到。
func assertMethodCapability(t *testing.T, manifest infoManifest, verb, want string) {
	t.Helper()
	key := "file." + verb
	got, ok := manifest.MethodCapabilities[key]
	if !ok {
		t.Fatalf("info.json 缺少方法级能力声明 %q", key)
	}
	if got != want {
		t.Fatalf("%s 能力=%q，期望 %q", key, got, want)
	}
	normalized, namespaced := businessVerb(key)
	if !namespaced || normalized != verb {
		t.Fatalf("businessVerb(%q)=(%q,%v)，与 info.json 声明不一致", key, normalized, namespaced)
	}
}
