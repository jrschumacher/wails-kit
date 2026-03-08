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

	if err := os.Rename(newPath, currentPath); err != nil {
		// Try to restore the old binary
		_ = os.Rename(oldPath, currentPath)
		return fmt.Errorf("install new binary: %w", err)
	}

	// Clean up the old binary (best effort)
	_ = os.Remove(oldPath)

	return nil
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
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
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
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
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
