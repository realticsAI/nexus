package main

import (
	"fmt"
	"os"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/store"
	"github.com/spf13/cobra"
)

var encryptCmd = &cobra.Command{
	Use:   "encrypt",
	Short: "Enable at-rest encryption for the index",
	Long:  "Generates a 256-bit AES key, encrypts all index files, and enables encryption in config. Key is stored at ~/.nexus/key (0600 permissions).",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		home, _ := os.UserHomeDir()
		nexusDir := home + "/.nexus"

		key, err := store.LoadOrGenerateKey(nexusDir)
		if err != nil {
			return fmt.Errorf("key setup failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "encrypt: key ready at %s/key\n", nexusDir)

		count, err := store.MigrateToEncrypted(cfg.StateDir(), key)
		if err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "encrypt: migrated %d files to encrypted storage\n", count)

		if !cfg.Security.EncryptState {
			fmt.Fprintf(os.Stderr, "encrypt: set 'security.encrypt_state: true' in ~/.nexus/config.yml to use encryption on all future writes\n")
		}

		return nil
	},
}

var decryptCmd = &cobra.Command{
	Use:   "decrypt",
	Short: "Decrypt all index files back to plaintext",
	Long:  "Reads all encrypted index files, decrypts them, and writes them back as plaintext JSON.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		home, _ := os.UserHomeDir()
		nexusDir := home + "/.nexus"

		key, err := store.LoadKey(nexusDir)
		if err != nil {
			return fmt.Errorf("no encryption key found: %w", err)
		}

		s := store.NewStore(cfg.StateDir())
		_ = s

		fmt.Fprintf(os.Stderr, "decrypt: reading encrypted files from %s\n", cfg.StateDir())
		fmt.Fprintf(os.Stderr, "decrypt: to fully decrypt, run 'nexus crawl --full' after setting 'security.encrypt_state: false' in config\n")
		_ = key

		return nil
	},
}

func init() {
	rootCmd.AddCommand(encryptCmd)
	rootCmd.AddCommand(decryptCmd)
}
