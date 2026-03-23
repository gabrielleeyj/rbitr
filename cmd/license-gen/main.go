package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/gabrielleeyj/rbitr/internal/license"
)

func main() {
	var (
		privateKeyPath string
		outputPath     string
		genKeypair     string
		licensee       string
		email          string
		tier           string
		expires        string
		keyVersion     int
		retentionDays  int
	)

	flag.StringVar(&privateKeyPath, "private-key", "", "path to Ed25519 private key PEM")
	flag.StringVar(&outputPath, "output", "license.key", "output path for the license key")
	flag.StringVar(&genKeypair, "gen-keypair", "", "generate a new Ed25519 keypair to the given directory")
	flag.StringVar(&licensee, "licensee", "", "licensee organization name (required)")
	flag.StringVar(&email, "email", "", "licensee contact email (required)")
	flag.StringVar(&tier, "tier", "paid", "license tier: free or paid")
	flag.StringVar(&expires, "expires", "", "expiry date in YYYY-MM-DD format (required)")
	flag.IntVar(&keyVersion, "key-version", license.CurrentKeyVersion, "license key format version")
	flag.IntVar(&retentionDays, "retention-days", 90, "audit retention days for paid tier")

	flag.Parse()

	if genKeypair != "" {
		generateKeypair(genKeypair)
		return
	}

	if privateKeyPath == "" || licensee == "" || email == "" || expires == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nRequired: --private-key, --licensee, --email, --expires")
		os.Exit(1)
	}

	if tier != "free" && tier != "paid" {
		log.Fatalf("invalid tier %q: must be 'free' or 'paid'", tier)
	}

	expiryTime, err := time.Parse(time.DateOnly, expires)
	if err != nil {
		log.Fatalf("invalid expiry date %q: use YYYY-MM-DD format", expires)
	}
	// Set to end of day.
	expiryTime = expiryTime.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	privKeyPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		log.Fatalf("reading private key: %v", err)
	}

	privKey, err := license.ParsePrivateKeyPEM(privKeyPEM)
	if err != nil {
		log.Fatalf("parsing private key: %v", err)
	}

	tokenBytes, err := buildLicenseToken(privKey, licensee, email, tier, keyVersion, retentionDays, expiryTime)
	if err != nil {
		log.Fatalf("building license token: %v", err)
	}

	if err := os.WriteFile(outputPath, tokenBytes, 0600); err != nil {
		log.Fatalf("writing license key: %v", err)
	}

	fmt.Printf("License key written to %s\n", outputPath)
	fmt.Printf("  Licensee:    %s (%s)\n", licensee, email)
	fmt.Printf("  Tier:        %s\n", tier)
	fmt.Printf("  Key version: %d\n", keyVersion)
	fmt.Printf("  Expires:     %s\n", expiryTime.Format(time.DateOnly))
	fmt.Printf("  Fingerprint: %s\n", license.Fingerprint(tokenBytes))
}

func buildLicenseToken(
	privKey ed25519.PrivateKey,
	licensee, email, tier string,
	keyVersion, retentionDays int,
	expiryTime time.Time,
) ([]byte, error) {
	ent := license.DefaultsForTier(tier)
	if retentionDays > 0 {
		ent.AuditRetentionDays = retentionDays
	}

	now := time.Now()
	tok, err := jwt.NewBuilder().
		Issuer("rbitr").
		Subject(licensee).
		IssuedAt(now).
		NotBefore(now).
		Expiration(expiryTime).
		Build()
	if err != nil {
		return nil, fmt.Errorf("building JWT: %w", err)
	}

	if err := tok.Set("key_version", keyVersion); err != nil {
		return nil, fmt.Errorf("setting key_version: %w", err)
	}
	if err := tok.Set("tier", tier); err != nil {
		return nil, fmt.Errorf("setting tier: %w", err)
	}
	if err := tok.Set("entitlements", ent); err != nil {
		return nil, fmt.Errorf("setting entitlements: %w", err)
	}
	if err := tok.Set("licensee", map[string]string{
		"name":  licensee,
		"email": email,
	}); err != nil {
		return nil, fmt.Errorf("setting licensee: %w", err)
	}

	key, err := jwk.Import(privKey)
	if err != nil {
		return nil, fmt.Errorf("importing private key: %w", err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.EdDSA(), key))
	if err != nil {
		return nil, fmt.Errorf("signing token: %w", err)
	}

	return signed, nil
}

func generateKeypair(dir string) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Fatalf("generating keypair: %v", err)
	}

	pubPEM, err := license.MarshalPublicKeyPEM(pub)
	if err != nil {
		log.Fatalf("marshaling public key: %v", err)
	}

	privPEM, err := license.MarshalPrivateKeyPEM(priv)
	if err != nil {
		log.Fatalf("marshaling private key: %v", err)
	}

	pubPath := filepath.Join(dir, "pubkey.pem")
	privPath := filepath.Join(dir, "private.pem")

	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		log.Fatalf("writing public key: %v", err)
	}
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		log.Fatalf("writing private key: %v", err)
	}

	fmt.Printf("Keypair generated:\n  Public:  %s\n  Private: %s\n", pubPath, privPath)
	fmt.Println("\nCopy the public key to internal/license/pubkey.pem and rebuild.")
	fmt.Println("Keep the private key SECURE — it signs all license keys.")
}
