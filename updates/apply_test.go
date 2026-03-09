package updates

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExtractTarGz(t *testing.T) {
	archivePath := createTestTarGz(t, map[string]string{
		"bin/myapp": "#!/bin/sh\necho hello",
		"README.md": "# readme",
	})

	dir, err := extractArchive(archivePath)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Check files exist
	content, err := os.ReadFile(filepath.Join(dir, "bin", "myapp"))
	if err != nil {
		t.Fatalf("read myapp: %v", err)
	}
	if string(content) != "#!/bin/sh\necho hello" {
		t.Errorf("unexpected content: %s", content)
	}

	content, err = os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if string(content) != "# readme" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestExtractZip(t *testing.T) {
	archivePath := createTestZip(t, map[string]string{
		"bin/myapp": "binary content",
		"LICENSE":   "MIT",
	})

	dir, err := extractArchive(archivePath)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	content, err := os.ReadFile(filepath.Join(dir, "bin", "myapp"))
	if err != nil {
		t.Fatalf("read myapp: %v", err)
	}
	if string(content) != "binary content" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestExtractArchive_PlainFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "myapp")
	if err := os.WriteFile(src, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	dir, err := extractArchive(src)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	content, err := os.ReadFile(filepath.Join(dir, "myapp"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(content) != "binary" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestExtractTarGz_PathTraversal(t *testing.T) {
	archivePath := createTestTarGzWithHeaders(t, []tarEntry{
		{name: "../etc/passwd", content: "root::0:0::"},
	})

	dir, err := extractArchive(archivePath)
	if err == nil {
		_ = os.RemoveAll(dir)
		t.Fatal("expected path traversal to be rejected")
	}
	if !strings.Contains(err.Error(), "escapes destination") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractZip_PathTraversal(t *testing.T) {
	archivePath := createTestZipWithNames(t, []zipEntry{
		{name: "../etc/passwd", content: "root::0:0::"},
	})

	dir, err := extractArchive(archivePath)
	if err == nil {
		_ = os.RemoveAll(dir)
		t.Fatal("expected path traversal to be rejected")
	}
	if !strings.Contains(err.Error(), "escapes destination") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractTarGz_ModeStripping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("setuid bits not applicable on Windows")
	}

	archivePath := createTestTarGzWithHeaders(t, []tarEntry{
		{name: "binary", content: "#!/bin/sh", mode: 0o4755}, // setuid bit
	})

	dir, err := extractArchive(archivePath)
	if err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	info, err := os.Stat(filepath.Join(dir, "binary"))
	if err != nil {
		t.Fatal(err)
	}

	mode := info.Mode().Perm()
	if mode&0o4000 != 0 {
		t.Errorf("setuid bit should be stripped, got mode %04o", mode)
	}
	if mode != 0o755 {
		t.Errorf("expected mode 0755, got %04o", mode)
	}
}

func TestFindBinary_ByName(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "myapp")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := findBinary(dir, "myapp")
	if err != nil {
		t.Fatalf("findBinary: %v", err)
	}
	if found != binPath {
		t.Errorf("expected %s, got %s", binPath, found)
	}
}

func TestFindBinary_InSubdir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "release")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(subdir, "myapp")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := findBinary(dir, "myapp")
	if err != nil {
		t.Fatalf("findBinary: %v", err)
	}
	if found != binPath {
		t.Errorf("expected %s, got %s", binPath, found)
	}
}

func TestFindBinary_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	_, err := findBinary(dir, "../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal in binaryName")
	}
	if !strings.Contains(err.Error(), "path separator") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFindBinary_FirstExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bit detection not reliable on Windows")
	}

	dir := t.TempDir()
	// Non-executable file
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Executable file
	if err := os.WriteFile(filepath.Join(dir, "myapp"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := findBinary(dir, "")
	if err != nil {
		t.Fatalf("findBinary: %v", err)
	}
	if filepath.Base(found) != "myapp" {
		t.Errorf("expected myapp, got %s", found)
	}
}

func TestFindBinary_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := findBinary(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error when binary not found")
	}
}

func TestDefaultApplier(t *testing.T) {
	// Set up "current" binary
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "current")
	if err := os.WriteFile(currentPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Set up "new" binary (in same dir to avoid cross-device)
	newPath := filepath.Join(dir, "new")
	if err := os.WriteFile(newPath, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	applier := defaultApplier{}
	if err := applier.Apply(newPath, currentPath); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	content, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Errorf("expected 'new', got %q", content)
	}

	// New file should have been cleaned up (renamed into currentPath)
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Error("expected newPath to be removed after apply")
	}
}

func TestMoveFile_SameDevice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile: %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "data" {
		t.Errorf("expected 'data', got %q", content)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("expected src to be removed")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("hello"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Errorf("expected 'hello', got %q", content)
	}
}

func TestSanitizeMode(t *testing.T) {
	tests := []struct {
		input os.FileMode
		want  os.FileMode
	}{
		{0o755, 0o755},
		{0o4755, 0o755},  // setuid stripped
		{0o2755, 0o755},  // setgid stripped
		{0o1755, 0o755},  // sticky stripped
		{0o7777, 0o777},  // all special bits stripped
		{0o644, 0o644},
	}

	for _, tt := range tests {
		got := sanitizeMode(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeMode(%04o) = %04o, want %04o", tt.input, got, tt.want)
		}
	}
}

// --- Test helpers ---

type tarEntry struct {
	name    string
	content string
	mode    int64
}

func createTestTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	var entries []tarEntry
	for name, content := range files {
		entries = append(entries, tarEntry{name: name, content: content, mode: 0o755})
	}
	return createTestTarGzWithHeaders(t, entries)
}

func createTestTarGzWithHeaders(t *testing.T, entries []tarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		hdr := &tar.Header{
			Name: e.name,
			Size: int64(len(e.content)),
			Mode: e.mode,
		}
		if strings.HasSuffix(e.name, "/") {
			hdr.Typeflag = tar.TypeDir
		} else {
			hdr.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(e.content)); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

type zipEntry struct {
	name    string
	content string
}

func createTestZip(t *testing.T, files map[string]string) string {
	t.Helper()
	var entries []zipEntry
	for name, content := range files {
		entries = append(entries, zipEntry{name: name, content: content})
	}
	return createTestZipWithNames(t, entries)
}

func createTestZipWithNames(t *testing.T, entries []zipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	w := zip.NewWriter(f)
	for _, e := range entries {
		fw, err := w.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
