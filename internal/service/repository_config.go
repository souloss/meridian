package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// RepositoryConfig is the normalized projection of one .asset-platform.yaml
// document after validation and root-dot normalization.
type RepositoryConfig struct {
	Version  int             `json:"version"`
	Services []ConfigService `json:"services"`
}

// ConfigService is one normalized service declaration.
type ConfigService struct {
	Name        string        `json:"name"`
	DisplayName string        `json:"displayName,omitempty"`
	Root        string        `json:"root"`
	Language    *string       `json:"language,omitempty"`
	Framework   *string       `json:"framework,omitempty"`
	Assets      []ConfigAsset `json:"assets"`
}

// ConfigAsset is one normalized asset declaration.
type ConfigAsset struct {
	Kind           string         `json:"kind"`
	Name           string         `json:"name,omitempty"`
	NamingTemplate string         `json:"namingTemplate,omitempty"`
	Base           *ConfigSource  `json:"base,omitempty"`
	Overlays       []ConfigSource `json:"overlays,omitempty"`
}

// ConfigSource is one normalized source declaration.
type ConfigSource struct {
	Mode            string   `json:"mode"`
	Origin          string   `json:"origin,omitempty"`
	Path            *string  `json:"path,omitempty"`
	ProducerProfile *string  `json:"producerProfile,omitempty"`
	Order           int      `json:"order,omitempty"`
	TimeoutSec      int      `json:"timeoutSec,omitempty"`
	BranchPatterns  []string `json:"branchPatterns,omitempty"`
	Enabled         *bool    `json:"enabled,omitempty"`
}

// ParseRepositoryConfig validates and normalizes one repository configuration
// document. The `.` service root is normalized to the empty string, and the
// service-root uniqueness / name-uniqueness invariants are enforced.
func ParseRepositoryConfig(content []byte) (RepositoryConfig, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return RepositoryConfig{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if len(root.Content) == 0 {
		return RepositoryConfig{}, ErrValidation
	}
	document := root.Content[0]
	if err := rejectDuplicateConfigKeys(document); err != nil {
		return RepositoryConfig{}, err
	}
	var raw struct {
		Version  int `yaml:"version"`
		Services []struct {
			Name        string  `yaml:"name"`
			DisplayName string  `yaml:"displayName"`
			Root        string  `yaml:"root"`
			Language    *string `yaml:"language"`
			Framework   *string `yaml:"framework"`
			Assets      []struct {
				Kind           string            `yaml:"kind"`
				Name           string            `yaml:"name"`
				NamingTemplate string            `yaml:"namingTemplate"`
				Base           *configSourceRaw  `yaml:"base"`
				Overlays       []configSourceRaw `yaml:"overlays"`
			} `yaml:"assets"`
		} `yaml:"services"`
	}
	if err := document.Decode(&raw); err != nil {
		return RepositoryConfig{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if raw.Version != 1 {
		return RepositoryConfig{}, ErrValidation
	}

	config := RepositoryConfig{Version: raw.Version, Services: make([]ConfigService, 0, len(raw.Services))}
	serviceNames := make(map[string]bool, len(raw.Services))
	serviceRoots := make(map[string]bool, len(raw.Services))
	for _, service := range raw.Services {
		name := service.Name
		if name == "" || serviceNames[name] {
			return RepositoryConfig{}, ErrValidation
		}
		serviceNames[name] = true
		rootDir := service.Root
		if rootDir == "." {
			rootDir = ""
		}
		if serviceRoots[rootDir] {
			return RepositoryConfig{}, ErrValidation
		}
		serviceRoots[rootDir] = true
		normalized := ConfigService{
			Name: name, DisplayName: service.DisplayName, Root: rootDir,
			Language: service.Language, Framework: service.Framework,
			Assets: make([]ConfigAsset, 0, len(service.Assets)),
		}
		assetNames := make(map[string]bool, len(service.Assets))
		for _, asset := range service.Assets {
			if asset.Kind == "" {
				return RepositoryConfig{}, ErrValidation
			}
			assetName := asset.Name
			if assetName != "" {
				if assetNames[assetName] {
					return RepositoryConfig{}, ErrValidation
				}
				assetNames[assetName] = true
			}
			normalizedAsset := ConfigAsset{Kind: asset.Kind, Name: assetName, NamingTemplate: asset.NamingTemplate}
			if asset.Base != nil {
				source, err := normalizeConfigSource(*asset.Base, false)
				if err != nil {
					return RepositoryConfig{}, err
				}
				normalizedAsset.Base = &source
			}
			for _, overlay := range asset.Overlays {
				source, err := normalizeConfigSource(overlay, true)
				if err != nil {
					return RepositoryConfig{}, err
				}
				normalizedAsset.Overlays = append(normalizedAsset.Overlays, source)
			}
			normalized.Assets = append(normalized.Assets, normalizedAsset)
		}
		config.Services = append(config.Services, normalized)
	}
	return config, nil
}

type configSourceRaw struct {
	Mode            string   `yaml:"mode"`
	Origin          string   `yaml:"origin"`
	Path            *string  `yaml:"path"`
	ProducerProfile *string  `yaml:"producerProfile"`
	Order           int      `yaml:"order"`
	TimeoutSec      int      `yaml:"timeoutSec"`
	BranchPatterns  []string `yaml:"branchPatterns"`
	Enabled         *bool    `yaml:"enabled"`
}

func normalizeConfigSource(raw configSourceRaw, isOverlay bool) (ConfigSource, error) {
	source := ConfigSource{
		Mode: raw.Mode, Origin: raw.Origin, Path: raw.Path, ProducerProfile: raw.ProducerProfile,
		Order: raw.Order, TimeoutSec: raw.TimeoutSec, BranchPatterns: raw.BranchPatterns, Enabled: raw.Enabled,
	}
	switch raw.Mode {
	case "builtin":
		if raw.Origin != "" && raw.Origin != "repo" {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path == nil || *raw.Path == "" {
			return ConfigSource{}, ErrValidation
		}
		if raw.ProducerProfile != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = "repo"
	case "command":
		if raw.Origin != "" && raw.Origin != "repo" {
			return ConfigSource{}, ErrValidation
		}
		if raw.ProducerProfile == nil || *raw.ProducerProfile == "" {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = "repo"
	case "push":
		if raw.Origin != "" && raw.Origin != "third_party" {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil || raw.ProducerProfile != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = "third_party"
	case "manual":
		if raw.Origin != "" && raw.Origin != "manual" {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil || raw.ProducerProfile != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = "manual"
	case "ai":
		if raw.Origin != "" && raw.Origin != "ai_generated" {
			return ConfigSource{}, ErrValidation
		}
		if raw.ProducerProfile == nil || *raw.ProducerProfile == "" {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = "ai_generated"
	default:
		return ConfigSource{}, ErrValidation
	}
	if source.TimeoutSec == 0 {
		source.TimeoutSec = defaultSourceTimeoutByMode(source.Mode)
	}
	if source.TimeoutSec < 10 || source.TimeoutSec > 3600 {
		return ConfigSource{}, ErrValidation
	}
	if len(source.BranchPatterns) == 0 {
		source.BranchPatterns = []string{"**"}
	}
	return source, nil
}

// ConfigDigest computes the lowercase SHA-256 digest of the canonical JSON form
// of a normalized repository configuration. Apply must receive the same digest.
func ConfigDigest(config RepositoryConfig) (string, error) {
	encoded, err := CanonicalJSON(config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// ConfigDigestFromBytes parses, normalizes, and digests one configuration document.
func ConfigDigestFromBytes(content []byte) (string, RepositoryConfig, error) {
	config, err := ParseRepositoryConfig(content)
	if err != nil {
		return "", RepositoryConfig{}, err
	}
	digest, err := ConfigDigest(config)
	if err != nil {
		return "", RepositoryConfig{}, err
	}
	return digest, config, nil
}

func rejectDuplicateConfigKeys(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		seen := make(map[string]bool, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index].Value
			if seen[key] {
				return fmt.Errorf("%w: duplicate key %q", ErrValidation, key)
			}
			seen[key] = true
			if err := rejectDuplicateConfigKeys(node.Content[index+1]); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := rejectDuplicateConfigKeys(child); err != nil {
				return err
			}
		}
	}
	return nil
}

var _ = json.Marshal
var _ = sort.Strings
var _ = strings.TrimSpace
var _ = utf8.RuneCountInString
