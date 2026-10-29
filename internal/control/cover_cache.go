package control

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"music-engine/internal/decoder"
)

const maxCoverBytes = 5 << 20
const maxCoverCacheBytes = 128 << 20

// Artwork runs independently of configuration and audio. Only local paths cross
// the desktop pipe; downloads never carry the device's bearer token.
func (r *Runtime) cacheCovers(ctx context.Context, dir string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		r.mu.Lock()
		urls := map[string]bool{}
		for _, raw := range r.config.ZoneBackgrounds {
			urls[raw] = true
		}
		for _, playlist := range r.config.Playlists {
			urls[playlist.CoverURL] = true
			for _, song := range playlist.Songs {
				urls[song.CoverURL] = true
			}
		}
		for _, zone := range r.config.Zones {
			if zone.Playlist == nil {
				continue
			}
			urls[zone.Playlist.CoverURL] = true
			for _, song := range zone.Playlist.Songs {
				urls[song.CoverURL] = true
			}
		}
		r.mu.Unlock()
		paths := map[string]string{}
		for raw := range urls {
			if ctx.Err() != nil {
				return
			}
			if !decoder.ValidURL(raw) {
				continue
			}
			if path, err := cacheCover(ctx, r.client.http, dir, raw); err == nil {
				paths[raw] = path
			}
		}
		r.mu.Lock()
		r.coverPaths = paths
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func cacheCover(ctx context.Context, transport *http.Client, dir, raw string) (string, error) {
	if !decoder.ValidURL(raw) {
		return "", fmt.Errorf("portada requiere HTTPS")
	}
	path, err := filepath.Abs(filepath.Join(dir, fmt.Sprintf("%x.img", sha256.Sum256([]byte(raw)))))
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > 0 && info.Size() <= maxCoverBytes {
		_ = os.Chtimes(path, time.Now(), time.Now())
		return path, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client := *transport
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "image/jpeg, image/png, image/webp")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("portada HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxCoverBytes {
		return "", fmt.Errorf("portada excede 5 MB")
	}
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return "", fmt.Errorf("formato de portada no compatible")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	trimCoverCache(dir, int64(len(data)))
	file, err := os.CreateTemp(dir, ".cover-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

func trimCoverCache(dir string, incoming int64) {
	entries, _ := os.ReadDir(dir)
	files := []os.FileInfo{}
	total := incoming
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".img" {
			continue
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, info)
			total += info.Size()
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().Before(files[j].ModTime()) })
	for _, file := range files {
		if total <= maxCoverCacheBytes {
			break
		}
		if os.Remove(filepath.Join(dir, file.Name())) == nil {
			total -= file.Size()
		}
	}
}
