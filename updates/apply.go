package updates

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxExtractSize is the maximum total bytes that can be extracted from an
// archive. This protects against decompression bombs.
const maxExtractSize = 1 << 30 // 1 GiB

// Applier replaces the running binary with a new version.
type Applier interface {
	Apply(newPath, currentPath string) error
}

// defaultApplier replaces a binary via atomic rename.
type defaultApplier struct{}

func (defaultApplier) Apply(newPath, currentPath string) error {
	// Ensure the new binary is executable
	if err := os.Chmod(newPath, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	// Atomic rename: move old to .old, new to current
	oldPath := currentPath + ".old"
	_ = os.Remove(oldPath) // clean up any previous .old

	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("backup current binary: %w", err)
	}

	if err := moveFile(newPath, currentPath); err != nil {
		// Try to restore the old binary
		_ = os.Rename(oldPath, currentPath)
		return fmt.Errorf("install new binary: %w", err)
	}

	// Clean up the old binary (best effort)
	_ = os.Remove(oldPath)

	return nil
}

// moveFile attempts os.Rename first, falling back to copy+remove for
// cross-filesystem moves (EXDEV).
func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	// If Rename failed for a reason other than cross-device, bail out.
	if !isCrossDeviceError(err) {
		return err
	}

	// Fall back to copy + remove.
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// extractArchive extracts a downloaded archive to a temp directory.
// Returns the path to the extracted directory. The caller is responsible
// for cleanup.
func extractArchive(archivePath string) (string, error) {
	lower := strings.ToLower(archivePath)

	dir, err := os.MkdirTemp("", "wails-kit-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		err = extractTarGz(archivePath, dir)
	case strings.HasSuffix(lower, ".zip"):
		err = extractZip(archivePath, dir)
	default:
		// Not an archive, just copy the file
		destPath := filepath.Join(dir, filepath.Base(archivePath))
		err = copyFile(archivePath, destPath)
	}

	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}

	return dir, nil
}

// sanitizeMode strips setuid, setgid, and sticky bits from archive-provided
// file modes so extracted files cannot gain unexpected privileges.
func sanitizeMode(mode os.FileMode) os.FileMode {
	return mode & 0o777
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	var totalBytes int64
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		target := filepath.Join(dest, header.Name)
		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %q escapes destination", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, sanitizeMode(os.FileMode(header.Mode)))
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(out, io.LimitReader(tr, maxExtractSize-totalBytes+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			totalBytes += written
			if totalBytes > maxExtractSize {
				return fmt.Errorf("archive exceeds maximum allowed size (%d bytes)", maxExtractSize)
			}
		}
	}

	return nil
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	var totalBytes int64
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("zip entry %q escapes destination", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, sanitizeMode(f.Mode()))
		if err != nil {
			_ = rc.Close()
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(rc, maxExtractSize-totalBytes+1))
		closeOutErr := out.Close()
		closeRcErr := rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeRcErr != nil {
			return closeRcErr
		}
		totalBytes += written
		if totalBytes > maxExtractSize {
			return fmt.Errorf("archive exceeds maximum allowed size (%d bytes)", maxExtractSize)
		}
	}

	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := in.Stat()
	if err != nil {
		return err
	}
	return os.Chmod(dest, info.Mode())
}

// findBinary locates the binary in an extracted directory.
// If binaryName is set, looks for that specific file.
// Otherwise returns the first executable file found.
func findBinary(dir, binaryName string) (string, error) {
	if binaryName != "" {
		// Reject path traversal in binaryName
		if strings.Contains(binaryName, string(os.PathSeparator)) || strings.Contains(binaryName, "/") ||
			binaryName == ".." || strings.Contains(binaryName, "../") {
			return "", fmt.Errorf("binary name %q contains path separator or traversal", binaryName)
		}

		path := filepath.Join(dir, binaryName)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		// Also check subdirectories one level deep
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				path = filepath.Join(dir, e.Name(), binaryName)
				if _, err := os.Stat(path); err == nil {
					return path, nil
				}
			}
		}
		return "", fmt.Errorf("binary %q not found in extracted archive", binaryName)
	}

	// Find the first executable file
	var found string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" {
			return err
		}
		if !info.IsDir() && info.Mode()&0o111 != 0 {
			found = path
		}
		return nil
	})

	if found == "" {
		return "", fmt.Errorf("no executable file found in extracted archive")
	}
	return found, nil
}
