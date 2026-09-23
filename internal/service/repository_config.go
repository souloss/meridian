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

// RepositoryConfig 是 .asset-platform.yaml 文档在校验与根点号规范化后的规范化投影。
type RepositoryConfig struct {
	// Version 承载 RepositoryConfig 的生成 Version 值。
	Version int `json:"version"`
	// Services 承载 RepositoryConfig 的生成 Services 值。
	Services []ConfigService `json:"services"`
}

// ConfigService 是一个规范化服务声明。
type ConfigService struct {
	// Name 承载 ConfigService 的生成 Name 值。
	Name string `json:"name"`
	// DisplayName 承载 ConfigService 的生成 DisplayName 值。
	DisplayName string `json:"displayName,omitempty"`
	// Root 承载 ConfigService 的生成 Root 值。
	Root string `json:"root"`
	// Language 承载 ConfigService 的生成 Language 值。
	Language *string `json:"language,omitempty"`
	// Framework 承载 ConfigService 的生成 Framework 值。
	Framework *string `json:"framework,omitempty"`
	// Assets 承载 ConfigService 的生成 Assets 值。
	Assets []ConfigAsset `json:"assets"`
}

// ConfigAsset 是一个规范化资产声明。
type ConfigAsset struct {
	// Kind 承载 ConfigAsset 的生成 Kind 值。
	Kind string `json:"kind"`
	// Name 承载 ConfigAsset 的生成 Name 值。
	Name string `json:"name,omitempty"`
	// NamingTemplate 承载 ConfigAsset 的生成 NamingTemplate 值。
	NamingTemplate string `json:"namingTemplate,omitempty"`
	// Base 承载 ConfigAsset 的生成 Base 值。
	Base *ConfigSource `json:"base,omitempty"`
	// Overlays 承载 ConfigAsset 的生成 Overlays 值。
	Overlays []ConfigSource `json:"overlays,omitempty"`
}

// ConfigSource 是一个规范化源声明。
type ConfigSource struct {
	// Mode 承载 ConfigSource 的生成 Mode 值。
	Mode string `json:"mode"`
	// Origin 承载 ConfigSource 的生成 Origin 值。
	Origin string `json:"origin,omitempty"`
	// Path 承载 ConfigSource 的生成 Path 值。
	Path *string `json:"path,omitempty"`
	// ProducerProfile 承载 ConfigSource 的生成 ProducerProfile 值。
	ProducerProfile *string `json:"producerProfile,omitempty"`
	// Order 承载 ConfigSource 的生成 Order 值。
	Order int `json:"order,omitempty"`
	// TimeoutSec 承载 ConfigSource 的生成 TimeoutSec 值。
	TimeoutSec int `json:"timeoutSec,omitempty"`
	// BranchPatterns 承载 ConfigSource 的生成 BranchPatterns 值。
	BranchPatterns []string `json:"branchPatterns,omitempty"`
	// Enabled 承载 ConfigSource 的生成 Enabled 值。
	Enabled *bool `json:"enabled,omitempty"`
}

// ParseRepositoryConfig 校验并规范化一个仓库配置文档。`.` 服务根规范化为空字符串，并强制
// 服务根唯一 / 名称唯一不变式。
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
	// Mode 承载 configSourceRaw 的生成 Mode 值。
	Mode string `yaml:"mode"`
	// Origin 承载 configSourceRaw 的生成 Origin 值。
	Origin string `yaml:"origin"`
	// Path 承载 configSourceRaw 的生成 Path 值。
	Path *string `yaml:"path"`
	// ProducerProfile 承载 configSourceRaw 的生成 ProducerProfile 值。
	ProducerProfile *string `yaml:"producerProfile"`
	// Order 承载 configSourceRaw 的生成 Order 值。
	Order int `yaml:"order"`
	// TimeoutSec 承载 configSourceRaw 的生成 TimeoutSec 值。
	TimeoutSec int `yaml:"timeoutSec"`
	// BranchPatterns 承载 configSourceRaw 的生成 BranchPatterns 值。
	BranchPatterns []string `yaml:"branchPatterns"`
	// Enabled 承载 configSourceRaw 的生成 Enabled 值。
	Enabled *bool `yaml:"enabled"`
}

func normalizeConfigSource(raw configSourceRaw, isOverlay bool) (ConfigSource, error) {
	source := ConfigSource{
		Mode: raw.Mode, Origin: raw.Origin, Path: raw.Path, ProducerProfile: raw.ProducerProfile,
		Order: raw.Order, TimeoutSec: raw.TimeoutSec, BranchPatterns: raw.BranchPatterns, Enabled: raw.Enabled,
	}
	switch raw.Mode {
	case sourceModeBuiltin:
		if raw.Origin != "" && raw.Origin != layerOriginRepo {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path == nil || *raw.Path == "" {
			return ConfigSource{}, ErrValidation
		}
		if raw.ProducerProfile != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = layerOriginRepo
	case sourceModeCommand:
		if raw.Origin != "" && raw.Origin != layerOriginRepo {
			return ConfigSource{}, ErrValidation
		}
		if raw.ProducerProfile == nil || *raw.ProducerProfile == "" {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = layerOriginRepo
	case sourceModePush:
		if raw.Origin != "" && raw.Origin != layerOriginThirdParty {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil || raw.ProducerProfile != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = layerOriginThirdParty
	case sourceModeManual:
		if raw.Origin != "" && raw.Origin != layerOriginManual {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil || raw.ProducerProfile != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = layerOriginManual
	case aiMode:
		if raw.Origin != "" && raw.Origin != aiOrigin {
			return ConfigSource{}, ErrValidation
		}
		if raw.ProducerProfile == nil || *raw.ProducerProfile == "" {
			return ConfigSource{}, ErrValidation
		}
		if raw.Path != nil {
			return ConfigSource{}, ErrValidation
		}
		source.Origin = aiOrigin
	default:
		return ConfigSource{}, ErrValidation
	}
	if source.TimeoutSec == 0 {
		source.TimeoutSec = defaultSourceTimeoutByMode(source.Mode)
	}
	if source.TimeoutSec < sourceTimeoutMinSec || source.TimeoutSec > sourceTimeoutMaxSec {
		return ConfigSource{}, ErrValidation
	}
	if len(source.BranchPatterns) == 0 {
		source.BranchPatterns = []string{sourceBranchPatternAll}
	}
	return source, nil
}

// ConfigDigest 计算规范化仓库配置的规范 JSON 形式的小写 SHA-256 摘要。Apply 必须收到相同摘要。
func ConfigDigest(config RepositoryConfig) (string, error) {
	encoded, err := CanonicalJSON(config)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// ConfigDigestFromBytes 解析、规范化并摘要一个配置文档。
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
