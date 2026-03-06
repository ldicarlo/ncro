package narinfo

import (
	"bufio"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parsed representation of a Nix narinfo file.
type NarInfo struct {
	StorePath   string
	URL         string
	Compression string
	FileHash    string
	FileSize    uint64
	NarHash     string
	NarSize     uint64
	References  []string
	Deriver     string
	Sig         []string
	CA          string
}

// ParsePublicKey parses a Nix public key in "name:base64(key)" format.
func ParsePublicKey(s string) (name string, key ed25519.PublicKey, err error) {
	name, b64, ok := strings.Cut(s, ":")
	if !ok || name == "" {
		return "", nil, fmt.Errorf("invalid public key %q: missing ':'", s)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", nil, fmt.Errorf("invalid public key %q: %w", s, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return "", nil, fmt.Errorf("invalid public key size %d, want %d", len(raw), ed25519.PublicKeySize)
	}
	return name, ed25519.PublicKey(raw), nil
}

// Fingerprint returns the canonical signing input for this narinfo.
// Format: 1;<storePath>;<narHash>;<narSize>;<comma-separated-full-ref-paths>
func (ni *NarInfo) Fingerprint() string {
	refs := make([]string, len(ni.References))
	for i, r := range ni.References {
		if strings.HasPrefix(r, "/nix/store/") {
			refs[i] = r
		} else {
			refs[i] = "/nix/store/" + r
		}
	}
	return fmt.Sprintf("1;%s;%s;%d;%s",
		ni.StorePath, ni.NarHash, ni.NarSize, strings.Join(refs, ","))
}

// Verify checks that at least one Sig line is a valid signature for pubKeyStr.
// pubKeyStr must be in "name:base64(key)" Nix format.
// Returns false (not an error) when no matching Sig line is found.
func (ni *NarInfo) Verify(pubKeyStr string) (bool, error) {
	keyName, key, err := ParsePublicKey(pubKeyStr)
	if err != nil {
		return false, err
	}
	fp := []byte(ni.Fingerprint())
	for _, sigLine := range ni.Sig {
		name, b64, ok := strings.Cut(sigLine, ":")
		if !ok || name != keyName {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(sig) != ed25519.SignatureSize {
			continue
		}
		if ed25519.Verify(key, fp, sig) {
			return true, nil
		}
	}
	return false, nil
}

// Parses a narinfo from r. Returns error on malformed input or missing StorePath.
func Parse(r io.Reader) (*NarInfo, error) {
	ni := &NarInfo{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			return nil, fmt.Errorf("malformed line: %q", line)
		}
		switch k {
		case "StorePath":
			ni.StorePath = v
		case "URL":
			ni.URL = v
		case "Compression":
			ni.Compression = v
		case "FileHash":
			ni.FileHash = v
		case "FileSize":
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("FileSize: %w", err)
			}
			ni.FileSize = n
		case "NarHash":
			ni.NarHash = v
		case "NarSize":
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("NarSize: %w", err)
			}
			ni.NarSize = n
		case "References":
			if v != "" {
				ni.References = strings.Fields(v)
			}
		case "Deriver":
			ni.Deriver = v
		case "Sig":
			ni.Sig = append(ni.Sig, v)
		case "CA":
			ni.CA = v
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if ni.StorePath == "" {
		return nil, fmt.Errorf("missing StorePath")
	}
	return ni, nil
}
