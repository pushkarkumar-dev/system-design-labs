package cicd

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Artifact represents a build output stored in the artifact store.
// Artifacts are content-addressed: two uploads of identical content
// return the same SHA256 and are stored only once.
type Artifact struct {
	Name      string
	Path      string
	SHA256    string
	Size      int64
	CreatedAt time.Time
}

// ArtifactStore is a content-addressed in-memory store for build artifacts.
// The map key is the SHA256 hex digest of the artifact content.
// Uploading the same content twice is a no-op — the existing artifact is returned.
type ArtifactStore struct {
	mu        sync.RWMutex
	artifacts map[string]*Artifact
}

// NewArtifactStore returns an empty ArtifactStore.
func NewArtifactStore() *ArtifactStore {
	return &ArtifactStore{
		artifacts: make(map[string]*Artifact),
	}
}

// Upload adds an artifact to the store.
// If content with the same SHA256 already exists, the existing artifact is returned
// and no new entry is created (deduplication).
// Pass content=nil to store by name+path without reading a file (useful in tests).
func (s *ArtifactStore) Upload(name, path string, content []byte) (*Artifact, error) {
	digest, size, err := hashContent(content, path)
	if err != nil {
		return nil, fmt.Errorf("artifact upload: hash %q: %w", path, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.artifacts[digest]; ok {
		return existing, nil
	}

	a := &Artifact{
		Name:      name,
		Path:      path,
		SHA256:    digest,
		Size:      size,
		CreatedAt: time.Now(),
	}
	s.artifacts[digest] = a
	return a, nil
}

// Get returns the artifact for the given SHA256 digest, or nil if not found.
func (s *ArtifactStore) Get(sha256 string) *Artifact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.artifacts[sha256]
}

// Len returns the number of unique artifacts in the store.
func (s *ArtifactStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.artifacts)
}

// hashContent computes the SHA256 of the provided bytes.
// If content is nil, it reads from the file at path.
func hashContent(content []byte, path string) (digest string, size int64, err error) {
	h := sha256.New()
	if content != nil {
		h.Write(content)
		return fmt.Sprintf("%x", h.Sum(nil)), int64(len(content)), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), n, nil
}

// HashBytes is a public helper: SHA256 hex digest of arbitrary bytes.
func HashBytes(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}
