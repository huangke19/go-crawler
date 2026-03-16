package main

import (
	"os"
	"testing"
)

// setupFileutilTest 为文件工具测试创建临时目录
func setupFileutilTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })
}

// TestLoadJSONFile 测试加载 JSON 文件
func TestLoadJSONFile(t *testing.T) {
	setupFileutilTest(t)

	// 创建测试 JSON 文件
	testData := `{"name": "test", "value": 123}`
	if err := os.WriteFile("test.json", []byte(testData), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 加载 JSON
	var result map[string]interface{}
	if err := loadJSONFile("test.json", &result); err != nil {
		t.Fatalf("加载 JSON 失败: %v", err)
	}

	// 验证数据
	if result["name"] != "test" {
		t.Errorf("name 不匹配: %v", result["name"])
	}
	if result["value"] != float64(123) {
		t.Errorf("value 不匹配: %v", result["value"])
	}
}

// TestLoadJSONFile_FileNotExists 测试文件不存在
func TestLoadJSONFile_FileNotExists(t *testing.T) {
	setupFileutilTest(t)

	var result map[string]interface{}
	err := loadJSONFile("nonexistent.json", &result)
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}
}

// TestLoadJSONFile_InvalidJSON 测试无效 JSON
func TestLoadJSONFile_InvalidJSON(t *testing.T) {
	setupFileutilTest(t)

	// 创建无效 JSON 文件
	if err := os.WriteFile("invalid.json", []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	var result map[string]interface{}
	err := loadJSONFile("invalid.json", &result)
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}
}

// TestSaveJSONFile 测试保存 JSON 文件
func TestSaveJSONFile(t *testing.T) {
	setupFileutilTest(t)

	// 准备测试数据
	data := map[string]interface{}{
		"name":  "test",
		"value": 456,
		"items": []string{"a", "b", "c"},
	}

	// 保存 JSON
	if err := saveJSONFile("output.json", data, 0644); err != nil {
		t.Fatalf("保存 JSON 失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat("output.json"); os.IsNotExist(err) {
		t.Fatal("文件未创建")
	}

	// 重新加载验证
	var loaded map[string]interface{}
	if err := loadJSONFile("output.json", &loaded); err != nil {
		t.Fatalf("加载 JSON 失败: %v", err)
	}

	if loaded["name"] != "test" {
		t.Errorf("name 不匹配")
	}
	if loaded["value"] != float64(456) {
		t.Errorf("value 不匹配")
	}
}

// TestSaveJSONFile_AtomicWrite 测试原子写入
func TestSaveJSONFile_AtomicWrite(t *testing.T) {
	setupFileutilTest(t)

	data := map[string]string{"key": "value"}

	// 第一次保存
	if err := saveJSONFile("atomic.json", data, 0644); err != nil {
		t.Fatalf("第一次保存失败: %v", err)
	}

	// 第二次保存（覆盖）
	data["key"] = "new_value"
	if err := saveJSONFile("atomic.json", data, 0644); err != nil {
		t.Fatalf("第二次保存失败: %v", err)
	}

	// 验证临时文件已清理
	if _, err := os.Stat("atomic.json.tmp"); err == nil {
		t.Error("临时文件未清理")
	}

	// 验证数据正确
	var loaded map[string]string
	if err := loadJSONFile("atomic.json", &loaded); err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if loaded["key"] != "new_value" {
		t.Error("数据未正确覆盖")
	}
}

// TestSaveJSONFile_Permissions 测试文件权限
func TestSaveJSONFile_Permissions(t *testing.T) {
	setupFileutilTest(t)

	data := map[string]string{"test": "data"}

	// 使用 0600 权限保存
	if err := saveJSONFile("secure.json", data, 0600); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 验证权限
	info, err := os.Stat("secure.json")
	if err != nil {
		t.Fatalf("获取文件信息失败: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("文件权限不正确: 期望 0600, 实际 %o", mode)
	}
}

// TestExtractShortcode_Extended 测试 shortcode 提取（扩展）
func TestExtractShortcode_Extended(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "标准帖子 URL",
			url:      "https://www.instagram.com/p/ABC123xyz/",
			expected: "ABC123xyz",
		},
		{
			name:     "Reel URL",
			url:      "https://www.instagram.com/reel/XYZ789abc/",
			expected: "XYZ789abc",
		},
		{
			name:     "带查询参数",
			url:      "https://www.instagram.com/p/DEF456/?utm_source=ig_web",
			expected: "DEF456",
		},
		{
			name:     "不带尾部斜杠",
			url:      "https://www.instagram.com/p/GHI789",
			expected: "GHI789",
		},
		{
			name:     "相对路径",
			url:      "/p/JKL012/",
			expected: "JKL012",
		},
		{
			name:     "无效 URL",
			url:      "https://www.instagram.com/user/profile/",
			expected: "",
		},
		{
			name:     "空 URL",
			url:      "",
			expected: "",
		},
		{
			name:     "只有域名",
			url:      "https://www.instagram.com/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractShortcode(tt.url)
			if result != tt.expected {
				t.Errorf("extractShortcode(%q) = %q, 期望 %q", tt.url, result, tt.expected)
			}
		})
	}
}

// TestExtractShortcodeFromPath_Extended 测试从路径提取 shortcode（扩展）
func TestExtractShortcodeFromPath_Extended(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{
			name:     "标准缓存路径",
			files:    []string{"downloads/cache/ABC123xyz/post_1.jpg"},
			expected: "ABC123xyz",
		},
		{
			name:     "多个文件",
			files:    []string{"downloads/cache/XYZ789/post_1.jpg", "downloads/cache/XYZ789/post_2.mp4"},
			expected: "XYZ789",
		},
		{
			name:     "Windows 路径（在 macOS 上不适用）",
			files:    []string{"downloads\\cache\\DEF456\\post_1.jpg"},
			expected: "", // macOS 使用 / 作为分隔符，\ 会被当作文件名的一部分
		},
		{
			name:     "用户目录路径",
			files:    []string{"downloads/nike/post_1.jpg"},
			expected: "",
		},
		{
			name:     "空路径列表",
			files:    []string{},
			expected: "",
		},
		{
			name:     "无效路径",
			files:    []string{"invalid/path"},
			expected: "",
		},
		{
			name:     "点号路径",
			files:    []string{"downloads/cache/./post_1.jpg"},
			expected: "",
		},
		{
			name:     "双点号路径",
			files:    []string{"downloads/cache/../post_1.jpg"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractShortcodeFromPath(tt.files)
			if result != tt.expected {
				t.Errorf("extractShortcodeFromPath(%v) = %q, 期望 %q", tt.files, result, tt.expected)
			}
		})
	}
}

// TestJSONRoundTrip 测试 JSON 往返转换
func TestJSONRoundTrip(t *testing.T) {
	setupFileutilTest(t)

	type TestStruct struct {
		Name   string   `json:"name"`
		Count  int      `json:"count"`
		Items  []string `json:"items"`
		Active bool     `json:"active"`
	}

	original := TestStruct{
		Name:   "test",
		Count:  42,
		Items:  []string{"a", "b", "c"},
		Active: true,
	}

	// 保存
	if err := saveJSONFile("roundtrip.json", original, 0644); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	// 加载
	var loaded TestStruct
	if err := loadJSONFile("roundtrip.json", &loaded); err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	// 验证
	if loaded.Name != original.Name {
		t.Errorf("Name 不匹配")
	}
	if loaded.Count != original.Count {
		t.Errorf("Count 不匹配")
	}
	if len(loaded.Items) != len(original.Items) {
		t.Errorf("Items 长度不匹配")
	}
	if loaded.Active != original.Active {
		t.Errorf("Active 不匹配")
	}
}
