package enterprise

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// KeyManager manages RSA keys for signing
type KeyManager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
	keyDir     string
	mu         sync.RWMutex
}

// NewKeyManager creates a new key manager
func NewKeyManager(keyDir string) (*KeyManager, error) {
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create key directory: %w", err)
	}

	km := &KeyManager{
		keyDir: keyDir,
		keyID:  "default",
	}

	// Try to load existing keys
	if err := km.LoadOrGenerate(); err != nil {
		return nil, fmt.Errorf("failed to load or generate keys: %w", err)
	}

	return km, nil
}

// LoadOrGenerate loads existing keys or generates new ones
func (km *KeyManager) LoadOrGenerate() error {
	privateKeyPath := filepath.Join(km.keyDir, "signing_key.pem")
	publicKeyPath := filepath.Join(km.keyDir, "signing_key_pub.pem")

	// Check if keys exist
	if _, err := os.Stat(privateKeyPath); err == nil {
		// Load existing keys
		if err := km.Load(privateKeyPath, publicKeyPath); err != nil {
			// Generate new keys if load fails
			return km.Generate()
		}
		return nil
	}

	// Generate new keys
	return km.Generate()
}

// Generate generates a new RSA key pair
func (km *KeyManager) Generate() error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Generate 4096-bit RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	km.privateKey = privateKey
	km.publicKey = &privateKey.PublicKey

	// Save keys
	privateKeyPath := filepath.Join(km.keyDir, "signing_key.pem")
	publicKeyPath := filepath.Join(km.keyDir, "signing_key_pub.pem")

	// Save private key
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(km.privateKey),
	})
	if err := os.WriteFile(privateKeyPath, privateKeyPEM, 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}

	// Save public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(km.publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	if err := os.WriteFile(publicKeyPath, publicKeyPEM, 0644); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	return nil
}

// Load loads keys from files
func (km *KeyManager) Load(privateKeyPath, publicKeyPath string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Load private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(privateKeyData)
	if block == nil {
		return fmt.Errorf("failed to decode private key PEM")
	}

	km.privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Load public key
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	block, _ = pem.Decode(publicKeyData)
	if block == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	km.publicKey = pub.(*rsa.PublicKey)

	return nil
}

// Sign signs data with the private key
func (km *KeyManager) Sign(data []byte) (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.privateKey == nil {
		return "", fmt.Errorf("no private key available")
	}

	// Hash the data
	hashed := sha256.Sum256(data)

	// Sign
	signature, err := rsa.SignPKCS1v15(rand.Reader, km.privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}

	// Base64 encode
	return base64.StdEncoding.EncodeToString(signature), nil
}

// Verify verifies a signature
func (km *KeyManager) Verify(data []byte, signature string) error {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.publicKey == nil {
		return fmt.Errorf("no public key available")
	}

	// Decode signature
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	// Hash the data
	hashed := sha256.Sum256(data)

	// Verify
	return rsa.VerifyPKCS1v15(km.publicKey, crypto.SHA256, hashed[:], sig)
}

// SignManifest signs a manifest and returns the signature
func (km *KeyManager) SignManifest(manifest *Manifest) error {
	// Create a canonical representation for signing
	data := fmt.Sprintf("%s:%s:%s:%d",
		manifest.ArtifactID,
		manifest.Version,
		manifest.SHA256,
		manifest.Size,
	)

	signature, err := km.Sign([]byte(data))
	if err != nil {
		return err
	}

	manifest.Signature = &ManifestSignature{
		Algorithm: "RS256",
		KeyID:     km.keyID,
		Value:     signature,
		SignedAt:  time.Now().UTC(),
	}

	return nil
}

// VerifyManifest verifies a manifest signature
func (km *KeyManager) VerifyManifest(manifest *Manifest) error {
	if manifest.Signature == nil {
		return fmt.Errorf("manifest is not signed")
	}

	data := fmt.Sprintf("%s:%s:%s:%d",
		manifest.ArtifactID,
		manifest.Version,
		manifest.SHA256,
		manifest.Size,
	)

	return km.Verify([]byte(data), manifest.Signature.Value)
}

// PublicKeyPEM returns the public key in PEM format
func (km *KeyManager) PublicKeyPEM() (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.publicKey == nil {
		return "", fmt.Errorf("no public key available")
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(km.publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}

// KeyID returns the current key ID
func (km *KeyManager) KeyID() string {
	return km.keyID
}

// SetKeyID sets the key ID
func (km *KeyManager) SetKeyID(keyID string) {
	km.mu.Lock()
	defer km.mu.Unlock()
	km.keyID = keyID
}

// Manifest represents a signed artifact manifest.
type Manifest struct {
	ArtifactID string             `json:"artifact_id"`
	Version    string             `json:"version"`
	SHA256     string             `json:"sha256"`
	Size       int64              `json:"size"`
	Signature  *ManifestSignature `json:"signature,omitempty"`
}

// ManifestSignature holds signature metadata.
type ManifestSignature struct {
	Algorithm string    `json:"algorithm"`
	KeyID     string    `json:"key_id"`
	Value     string    `json:"value"`
	SignedAt  time.Time `json:"signed_at"`
}
