package main

import (
	"os"
	"path/filepath"
	"testing"
)

// setupAuthTest 为认证测试创建临时目录
func setupAuthTest(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(oldDir) })
	return tmpDir
}

// TestLoadSession_FileNotExists 测试会话文件不存在时的行为
func TestLoadSession_FileNotExists(t *testing.T) {
	setupAuthTest(t)

	_, err := LoadSession()
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}
}

// TestSaveAndLoadSession 测试保存和加载会话
func TestSaveAndLoadSession(t *testing.T) {
	setupAuthTest(t)

	// 准备测试数据
	cookies := []*Cookie{
		{
			Name:     "sessionid",
			Value:    "test_session_123",
			Domain:   ".instagram.com",
			Path:     "/",
			Expires:  1234567890.0,
			HTTPOnly: true,
			Secure:   true,
		},
		{
			Name:   "csrftoken",
			Value:  "test_csrf_456",
			Domain: ".instagram.com",
			Path:   "/",
		},
	}

	// 保存会话
	if err := SaveSession(cookies); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		t.Fatal("会话文件未创建")
	}

	// 加载会话
	loaded, err := LoadSession()
	if err != nil {
		t.Fatalf("加载会话失败: %v", err)
	}

	// 验证数据
	if len(loaded) != len(cookies) {
		t.Fatalf("Cookie 数量不匹配: 期望 %d, 实际 %d", len(cookies), len(loaded))
	}

	for i, cookie := range loaded {
		if cookie.Name != cookies[i].Name {
			t.Errorf("Cookie[%d] Name 不匹配: 期望 %s, 实际 %s", i, cookies[i].Name, cookie.Name)
		}
		if cookie.Value != cookies[i].Value {
			t.Errorf("Cookie[%d] Value 不匹配: 期望 %s, 实际 %s", i, cookies[i].Value, cookie.Value)
		}
		if cookie.Domain != cookies[i].Domain {
			t.Errorf("Cookie[%d] Domain 不匹配: 期望 %s, 实际 %s", i, cookies[i].Domain, cookie.Domain)
		}
	}
}

// TestSaveSession_FilePermissions 测试会话文件权限
func TestSaveSession_FilePermissions(t *testing.T) {
	setupAuthTest(t)

	cookies := []*Cookie{
		{Name: "test", Value: "value"},
	}

	if err := SaveSession(cookies); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	// 检查文件权限
	info, err := os.Stat(sessionFile)
	if err != nil {
		t.Fatalf("获取文件信息失败: %v", err)
	}

	// 验证权限为 0600 (仅所有者可读写)
	mode := info.Mode().Perm()
	expected := os.FileMode(0600)
	if mode != expected {
		t.Errorf("文件权限不正确: 期望 %o, 实际 %o", expected, mode)
	}
}

// TestSaveSession_EmptyCookies 测试保存空 Cookie 列表
func TestSaveSession_EmptyCookies(t *testing.T) {
	setupAuthTest(t)

	cookies := []*Cookie{}

	if err := SaveSession(cookies); err != nil {
		t.Fatalf("保存空会话失败: %v", err)
	}

	loaded, err := LoadSession()
	if err != nil {
		t.Fatalf("加载空会话失败: %v", err)
	}

	if len(loaded) != 0 {
		t.Errorf("期望空列表，但得到 %d 个 Cookie", len(loaded))
	}
}

// TestSaveSession_OverwriteExisting 测试覆盖现有会话文件
func TestSaveSession_OverwriteExisting(t *testing.T) {
	setupAuthTest(t)

	// 第一次保存
	cookies1 := []*Cookie{
		{Name: "old", Value: "value1"},
	}
	if err := SaveSession(cookies1); err != nil {
		t.Fatalf("第一次保存失败: %v", err)
	}

	// 第二次保存（覆盖）
	cookies2 := []*Cookie{
		{Name: "new", Value: "value2"},
	}
	if err := SaveSession(cookies2); err != nil {
		t.Fatalf("第二次保存失败: %v", err)
	}

	// 验证加载的是新数据
	loaded, err := LoadSession()
	if err != nil {
		t.Fatalf("加载会话失败: %v", err)
	}

	if len(loaded) != 1 || loaded[0].Name != "new" {
		t.Error("会话未正确覆盖")
	}
}

// TestLoadSession_CorruptedFile 测试损坏的会话文件
func TestLoadSession_CorruptedFile(t *testing.T) {
	setupAuthTest(t)

	// 创建损坏的 JSON 文件
	if err := os.WriteFile(sessionFile, []byte("invalid json {{{"), 0600); err != nil {
		t.Fatalf("创建损坏文件失败: %v", err)
	}

	_, err := LoadSession()
	if err == nil {
		t.Fatal("期望返回错误，但成功了")
	}
}

// TestSessionFile_Location 测试会话文件位置
func TestSessionFile_Location(t *testing.T) {
	tmpDir := setupAuthTest(t)

	cookies := []*Cookie{{Name: "test", Value: "value"}}
	if err := SaveSession(cookies); err != nil {
		t.Fatalf("保存会话失败: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, sessionFile)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("会话文件未在预期位置创建: %s", expectedPath)
	}
}
