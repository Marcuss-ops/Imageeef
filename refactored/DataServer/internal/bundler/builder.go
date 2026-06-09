package bundler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type BuilderOptions struct {
	SourceDir string
	OutputDir string
	UseDocker bool
}

type Builder struct {
	SourceDir string
	OutputDir string
	UseDocker bool
}

func NewBuilder(opts BuilderOptions) *Builder {
	return &Builder{
		SourceDir: opts.SourceDir,
		OutputDir: opts.OutputDir,
		UseDocker: opts.UseDocker,
	}
}

type ChunkInfo struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type ManifestV2 struct {
	Version   string               `json:"version"`
	BuildHash string               `json:"build_hash"`
	Timestamp string               `json:"timestamp"`
	Chunks    map[string]ChunkInfo `json:"chunks"`
}

func (b *Builder) BuildAllChunks() error {
	// === STEP 0: Validate required artifacts exist ===
	log.Printf("🔍 Validating required artifacts...")
	validation, err := b.ValidateArtifacts()
	if err != nil {
		return fmt.Errorf("artifact validation failed: %w", err)
	}

	// Write validation report
	validationPath := filepath.Join(b.OutputDir, "artifact_validation.json")
	if err := validation.WriteValidationReport(validationPath); err != nil {
		log.Printf("⚠️  Failed to write validation report: %v", err)
	} else {
		log.Printf("📄 Validation report: %s", validationPath)
	}

	// Print summary
	validation.PrintSummary()

	if !validation.Valid {
		return fmt.Errorf("❌ bundle build aborted: %d required artifacts missing or invalid (see artifact_validation.json)",
			validation.Summary["missing"]+validation.Summary["too_small"])
	}

	if b.UseDocker {
		log.Printf("🐳 Executing Docker Pre-Build for native Ubuntu dependencies...")
		scriptPath := filepath.Join(b.SourceDir, "DataServer", "cmd", "velox-bundler", "docker_prebuild.sh")

		cmd := exec.Command("/bin/bash", scriptPath, b.SourceDir)
		cmd.Dir = b.SourceDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("docker pre-build failed: %v", err)
		}
		log.Printf("🐳 Docker Pre-Build finished successfully!")
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := []error{}
	chunksInfo := make(map[string]ChunkInfo)

	tasks := []struct {
		name    string
		buildFn func() (string, int64, error)
	}{
		{"code", b.buildCodeChunk},
		{"venv", b.buildVenvChunk},
	}

	for _, t := range tasks {
		wg.Add(1)
		go func(taskName string, fn func() (string, int64, error)) {
			defer wg.Done()
			log.Printf("📦 Building chunk: %s", taskName)
			hash, size, err := fn()

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("chunk %s failed: %w", taskName, err))
			} else {
				chunksInfo[taskName] = ChunkInfo{
					File:   fmt.Sprintf("chunk_%s.zip", taskName),
					SHA256: hash,
					Size:   size,
				}
				log.Printf("✅ Chunk %s complete: %s (%.2f MB)", taskName, hash[:16], float64(size)/1024/1024)
			}
		}(t.name, t.buildFn)
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors during build: %v", len(errs), errs)
	}

	// Legacy bundle compatibility: merge all chunks into one worker_code.zip
	legacyBundlePath := filepath.Join(b.OutputDir, "worker_code.zip")
	chunksToMerge := []string{
		filepath.Join(b.OutputDir, "chunk_code.zip"),
		filepath.Join(b.OutputDir, "chunk_venv.zip"),
	}

	if err := os.Remove(legacyBundlePath); err != nil && !os.IsNotExist(err) {
		log.Printf("⚠️ Failed to remove old legacy zip: %v", err)
	}

	log.Printf("📦 Merging chunks into legacy worker bundle...")
	if err := mergeZips(legacyBundlePath, chunksToMerge); err != nil {
		log.Printf("⚠️ Failed to merge chunks for legacy bundle: %v", err)
	} else {
		log.Printf("✅ Backwards-compatible worker_code.zip generated (merged all chunks)")
	}

	// Generate Manifest
	return b.generateManifest(chunksInfo)
}

func (b *Builder) generateManifest(chunks map[string]ChunkInfo) error {
	version := b.getVersion()

	// Compute a global build hash from chunk hashes
	h := sha256.New()
	for _, chunkName := range []string{"code", "venv"} {
		if c, ok := chunks[chunkName]; ok {
			h.Write([]byte(c.SHA256))
		}
	}
	globalHash := hex.EncodeToString(h.Sum(nil))

	manifest := ManifestV2{
		Version:   version,
		BuildHash: globalHash,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Chunks:    chunks,
	}

	outPath := filepath.Join(b.OutputDir, "manifest_v2.json")
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return err
	}

	log.Printf("✅ Manifest created: %s (Build Hash: %s)", outPath, globalHash[:16])
	return nil
}

func (b *Builder) getVersion() string {
	candidates := []string{
		filepath.Join(b.SourceDir, "VERSION.txt"),
		filepath.Join(b.SourceDir, "refactored", "VERSION.txt"),
		filepath.Join(b.SourceDir, "config", "version", "VERSION.txt"),
	}
	for _, c := range candidates {
		if b, err := os.ReadFile(c); err == nil {
			v := strings.TrimSpace(strings.Split(string(b), "\n")[0])
			if v != "" {
				return v
			}
		}
	}

	// Fallback git commit
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = b.SourceDir
	if out, err := cmd.Output(); err == nil {
		return "git-" + strings.TrimSpace(string(out))
	}

	return "unknown"
}
