package source

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// HTTPProvider handles HTTP downloads (tar.gz, zip)
type HTTPProvider struct{}

func init() {
	RegisterProvider(&HTTPProvider{})
}

func (p *HTTPProvider) Type() string {
	return "http"
}

func (p *HTTPProvider) CanHandle(url string) bool {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		lower := strings.ToLower(url)
		return strings.HasSuffix(lower, ".tar.gz") ||
			strings.HasSuffix(lower, ".tgz") ||
			strings.HasSuffix(lower, ".zip")
	}
	return false
}

func (p *HTTPProvider) Fetch(ctx context.Context, url string, destPath string, opts FetchOptions) error {
	tmpFile, err := os.CreateTemp("", "ccp-download-*")
	if err != nil {
		return &SourceError{Op: "http download", Source: url, Err: err}
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return &SourceError{Op: "http download", Source: url, Err: err}
	}

	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &SourceError{Op: "http download", Source: url, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &SourceError{Op: "http download", Source: url,
			Err: fmt.Errorf("status %d", resp.StatusCode)}
	}

	hash := sha256.New()
	writer := io.MultiWriter(tmpFile, hash)
	if _, err := io.Copy(writer, resp.Body); err != nil {
		return &SourceError{Op: "http download", Source: url, Err: err}
	}

	if _, err := tmpFile.Seek(0, 0); err != nil {
		return &SourceError{Op: "http extract", Source: url, Err: err}
	}

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return &SourceError{Op: "http extract", Source: url, Err: err}
	}

	lower := strings.ToLower(url)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		if err := extractTarGz(tmpFile, destPath); err != nil {
			return &SourceError{Op: "http extract", Source: url, Err: err}
		}
	} else if strings.HasSuffix(lower, ".zip") {
		if err := extractZip(tmpFile.Name(), destPath); err != nil {
			return &SourceError{Op: "http extract", Source: url, Err: err}
		}
	}

	return nil
}

func (p *HTTPProvider) Update(ctx context.Context, sourcePath string, opts UpdateOptions) (*UpdateResult, error) {
	return &UpdateResult{Updated: false}, nil
}

func extractTarGz(r io.Reader, destPath string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var topDir string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		parts := strings.SplitN(header.Name, "/", 2)
		if topDir == "" && len(parts) > 1 {
			topDir = parts[0]
		}

		name := header.Name
		if topDir != "" && strings.HasPrefix(name, topDir+"/") {
			name = strings.TrimPrefix(name, topDir+"/")
		}
		if name == "" {
			continue
		}

		target := filepath.Join(destPath, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}

	return nil
}

func extractZip(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	var topDir string
	for _, f := range r.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if topDir == "" && len(parts) > 1 && f.FileInfo().IsDir() {
			topDir = parts[0]
			break
		}
	}

	for _, f := range r.File {
		name := f.Name
		if topDir != "" && strings.HasPrefix(name, topDir+"/") {
			name = strings.TrimPrefix(name, topDir+"/")
		}
		if name == "" {
			continue
		}

		target := filepath.Join(destPath, name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, f.Mode())
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
