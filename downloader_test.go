package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestGetFileExtension 测试从 URL 提取文件扩展名
func TestGetFileExtension(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"/foo/bar.mp4", ".mp4"},
		{"/foo/bar.jpg?se=123&token=abc", ".jpg"},
		{"/foo/bar.png", ".png"},
		{"/foo/bar", ".jpg"}, // 无扩展名 → 默认 .jpg
		{"https://example.com/media/video.mp4?size=hd", ".mp4"},
	}

	for _, c := range cases {
		got := GetFileExtension(c.url)
		if got != c.want {
			t.Errorf("GetFileExtension(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

// TestDownloadMedia_Success 测试正常下载：200 响应 → 文件写入磁盘
func TestDownloadMedia_Success(t *testing.T) {
	content := []byte("fake image data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	savePath := filepath.Join(t.TempDir(), "test.jpg")
	if err := DownloadMedia(srv.URL+"/img.jpg", savePath, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content = %q, want %q", got, content)
	}
}

// TestDownloadMedia_404 测试 404 响应 → 返回错误
func TestDownloadMedia_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	savePath := filepath.Join(t.TempDir(), "test.jpg")
	err := DownloadMedia(srv.URL+"/missing.jpg", savePath, 0)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// TestDownloadMedia_Retry 测试重试逻辑：第一次 500，第二次 200 → 最终成功
func TestDownloadMedia_Retry(t *testing.T) {
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	savePath := filepath.Join(t.TempDir(), "retry.jpg")
	if err := DownloadMedia(srv.URL+"/img.jpg", savePath, 1); err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if attempt != 2 {
		t.Errorf("expected 2 attempts, got %d", attempt)
	}
}

// TestDownloadConcurrently_AllSuccess 测试全部成功 → 无错误
func TestDownloadConcurrently_AllSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	tasks := []downloadTask{
		{url: srv.URL + "/a.jpg", savePath: filepath.Join(dir, "a.jpg")},
		{url: srv.URL + "/b.jpg", savePath: filepath.Join(dir, "b.jpg")},
		{url: srv.URL + "/c.mp4", savePath: filepath.Join(dir, "c.mp4")},
	}

	if err := downloadConcurrently(tasks, 2, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, task := range tasks {
		if _, err := os.Stat(task.savePath); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", task.savePath)
		}
	}
}

// TestDownloadConcurrently_PartialFailure 测试部分失败 → 返回错误且不挂死
func TestDownloadConcurrently_PartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad.jpg" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	tasks := []downloadTask{
		{url: srv.URL + "/good.jpg", savePath: filepath.Join(dir, "good.jpg")},
		{url: srv.URL + "/bad.jpg", savePath: filepath.Join(dir, "bad.jpg")},
	}

	err := downloadConcurrently(tasks, 2, 0)
	if err == nil {
		t.Fatal("expected error for partial failure, got nil")
	}
}

// TestDownloadPostInternal_FileNaming 测试文件命名规则（最重要的保护点）
func TestDownloadPostInternal_FileNaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("media"))
	}))
	defer srv.Close()

	// 切换到临时目录，隔离文件系统（Go 1.24 支持 t.Chdir）
	t.Chdir(t.TempDir())

	cases := []struct {
		name      string
		username  string
		postIndex int
		media     *MediaInfo
		wantFiles []string
	}{
		{
			name:      "单图",
			username:  "user1",
			postIndex: 1,
			media: &MediaInfo{
				Type: "image",
				URLs: []string{srv.URL + "/img.jpg"},
			},
			wantFiles: []string{"downloads/user1/post_1.jpg"},
		},
		{
			name:      "单视频",
			username:  "user2",
			postIndex: 2,
			media: &MediaInfo{
				Type: "video",
				URLs: []string{srv.URL + "/video.mp4"},
			},
			wantFiles: []string{"downloads/user2/post_2.mp4"},
		},
		{
			name:      "轮播（图+视频+图）",
			username:  "user3",
			postIndex: 3,
			media: &MediaInfo{
				Type:  "carousel",
				URLs:  []string{srv.URL + "/1.jpg", srv.URL + "/2.mp4", srv.URL + "/3.jpg"},
				Types: []string{"image", "video", "image"},
			},
			wantFiles: []string{
				"downloads/user3/post_3_1.jpg",
				"downloads/user3/post_3_2.mp4",
				"downloads/user3/post_3_3.jpg",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			paths, err := downloadPostInternal(c.username, c.postIndex, c.media)
			if err != nil {
				t.Fatalf("downloadPostInternal error: %v", err)
			}

			if len(paths) != len(c.wantFiles) {
				t.Fatalf("got %d paths, want %d", len(paths), len(c.wantFiles))
			}

			for i, want := range c.wantFiles {
				got := filepath.ToSlash(paths[i])
				if got != want {
					t.Errorf("paths[%d] = %q, want %q", i, got, want)
				}
				if _, err := os.Stat(paths[i]); os.IsNotExist(err) {
					t.Errorf("file %s not created", paths[i])
				}
			}
		})
	}
}

// TestDownloadPostInternal_MultiImage 测试多图（image 类型多 URL）的命名规则
func TestDownloadPostInternal_MultiImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "img")
	}))
	defer srv.Close()

	t.Chdir(t.TempDir())

	media := &MediaInfo{
		Type: "image",
		URLs: []string{srv.URL + "/1.jpg", srv.URL + "/2.jpg"},
	}

	paths, err := downloadPostInternal("multiuser", 5, media)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"downloads/multiuser/post_5_1.jpg",
		"downloads/multiuser/post_5_2.jpg",
	}
	for i, w := range want {
		got := filepath.ToSlash(paths[i])
		if got != w {
			t.Errorf("paths[%d] = %q, want %q", i, got, w)
		}
	}
}
