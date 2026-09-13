package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func keyPath(nexusDir string) string {
	return filepath.Join(nexusDir, "key")
}

func GenerateKey(nexusDir string) ([32]byte, error) {
	var key [32]byte
	p := keyPath(nexusDir)
	if _, err := os.Stat(p); err == nil {
		return key, fmt.Errorf("key file already exists: %s", p)
	}
	if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
		return key, fmt.Errorf("generating random key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return key, fmt.Errorf("creating key directory: %w", err)
	}
	if err := os.WriteFile(p, key[:], 0600); err != nil {
		return key, fmt.Errorf("writing key file: %w", err)
	}
	return key, nil
}

func LoadKey(nexusDir string) ([32]byte, error) {
	var key [32]byte
	data, err := os.ReadFile(keyPath(nexusDir))
	if err != nil {
		return key, fmt.Errorf("reading key file: %w", err)
	}
	if len(data) != 32 {
		return key, fmt.Errorf("invalid key file: expected 32 bytes, got %d", len(data))
	}
	copy(key[:], data)
	return key, nil
}

func LoadOrGenerateKey(nexusDir string) ([32]byte, error) {
	key, err := LoadKey(nexusDir)
	if err == nil {
		return key, nil
	}
	return GenerateKey(nexusDir)
}

func newGCM(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	return gcm, nil
}

func EncryptedWriteJSON(path string, v any, key [32]byte) error {
	plaintext, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	gcm, err := newGCM(key)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, ciphertext, 0600); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

func EncryptedReadJSON[T any](path string, key [32]byte) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize+gcm.Overhead() {
		return nil, fmt.Errorf("encrypted file too short: %d bytes", len(data))
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong key or tampered data): %w", err)
	}

	var v T
	if err := json.Unmarshal(plaintext, &v); err != nil {
		return nil, fmt.Errorf("unmarshaling decrypted JSON: %w", err)
	}
	return &v, nil
}

func MigrateToEncrypted(stateDir string, key [32]byte) (int, error) {
	migrated := 0

	err := filepath.Walk(stateDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}
		if info.Name() == "health.json" {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		if !json.Valid(data) {
			return nil
		}

		var generic any
		if err := json.Unmarshal(data, &generic); err != nil {
			return nil
		}

		if err := EncryptedWriteJSON(path, generic, key); err != nil {
			return fmt.Errorf("encrypting %s: %w", path, err)
		}
		migrated++
		return nil
	})

	return migrated, err
}
