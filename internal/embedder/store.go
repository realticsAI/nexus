package embedder

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	dir     string
	cache   map[string]ServiceVectors
	mu      sync.RWMutex
	maxSize int
	order   []string
}

type ServiceVectors struct {
	Vectors [][]float32
	Labels  []string
}

func NewStore(dir string, maxCacheSize int) *Store {
	if maxCacheSize <= 0 {
		maxCacheSize = 200
	}
	os.MkdirAll(dir, 0755)
	return &Store{
		dir:     dir,
		cache:   make(map[string]ServiceVectors),
		maxSize: maxCacheSize,
	}
}

func (s *Store) Save(serviceKey string, vectors [][]float32, labels []string) error {
	path := s.path(serviceKey)
	os.MkdirAll(filepath.Dir(path), 0755)

	f, err := os.Create(path + ".tmp")
	if err != nil {
		return err
	}
	defer f.Close()

	dimBytes := make([]byte, 4)
	if len(vectors) > 0 {
		binary.LittleEndian.PutUint32(dimBytes, uint32(len(vectors[0])))
	}
	f.Write(dimBytes)

	countBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(countBytes, uint32(len(vectors)))
	f.Write(countBytes)

	for _, vec := range vectors {
		for _, v := range vec {
			bits := math.Float32bits(v)
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, bits)
			f.Write(b)
		}
	}
	f.Close()
	if err := os.Rename(path+".tmp", path); err != nil {
		return err
	}

	s.mu.Lock()
	s.cache[serviceKey] = ServiceVectors{Vectors: vectors, Labels: labels}
	s.evictIfNeeded()
	s.mu.Unlock()
	return nil
}

func (s *Store) Load(serviceKey string) (ServiceVectors, bool) {
	s.mu.RLock()
	if sv, ok := s.cache[serviceKey]; ok {
		s.mu.RUnlock()
		return sv, true
	}
	s.mu.RUnlock()

	path := s.path(serviceKey)
	data, err := os.ReadFile(path)
	if err != nil {
		return ServiceVectors{}, false
	}

	if len(data) < 8 {
		return ServiceVectors{}, false
	}

	dim := int(binary.LittleEndian.Uint32(data[0:4]))
	count := int(binary.LittleEndian.Uint32(data[4:8]))
	offset := 8

	vectors := make([][]float32, count)
	for i := 0; i < count; i++ {
		vec := make([]float32, dim)
		for j := 0; j < dim; j++ {
			if offset+4 > len(data) {
				break
			}
			bits := binary.LittleEndian.Uint32(data[offset : offset+4])
			vec[j] = math.Float32frombits(bits)
			offset += 4
		}
		vectors[i] = vec
	}

	sv := ServiceVectors{Vectors: vectors}
	s.mu.Lock()
	s.cache[serviceKey] = sv
	s.evictIfNeeded()
	s.mu.Unlock()
	return sv, true
}

func (s *Store) evictIfNeeded() {
	for len(s.cache) > s.maxSize && len(s.order) > 0 {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.cache, oldest)
	}
}

func (s *Store) path(serviceKey string) string {
	safe := filepath.Base(serviceKey)
	return filepath.Join(s.dir, safe+".bin")
}

func (s *Store) Dir() string { return s.dir }
