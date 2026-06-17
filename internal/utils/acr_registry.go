package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"go.uber.org/zap"
)

func systemConfigValue(key string) string {
	var c models.SystemConfig
	if err := database.DB.Where("config_key = ?", key).First(&c).Error; err != nil {
		return ""
	}
	return c.ConfigValue
}

func authFromCredentials(username, rawPassword string) authn.Authenticator {
	if username == "" {
		return authn.Anonymous
	}
	pass, err := DecryptSystemConfigValue(rawPassword)
	if err != nil {
		logger.Logger.Warn("解密注册表密码失败，将尝试明文密码", zap.Error(err))
		pass = rawPassword
	}
	if pass == "" {
		return authn.Anonymous
	}
	return &authn.Basic{Username: username, Password: pass}
}

// getACRAuth returns an authenticator for the system default ACR config, or anonymous if unset.
func getACRAuth() authn.Authenticator {
	return authFromCredentials(
		systemConfigValue("aliyun_username"),
		systemConfigValue("aliyun_password"),
	)
}

// getACRAuthByRegistryID returns credentials for the given ACR registry ID, falling back to system config.
func getACRAuthByRegistryID(acrRegistryID uint) authn.Authenticator {
	if acrRegistryID > 0 {
		var acr models.AcrRegistry
		if err := database.DB.First(&acr, acrRegistryID).Error; err != nil {
			logger.Logger.Warn("获取ACR配置失败，回退到系统默认凭据",
				zap.Uint("acr_registry_id", acrRegistryID),
				zap.Error(err))
		} else {
			return authFromCredentials(acr.Username, acr.Password)
		}
	}
	return getACRAuth()
}

func remoteOptionsForACR(ctx context.Context, acrRegistryID uint) []remote.Option {
	return []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuth(getACRAuthByRegistryID(acrRegistryID)),
	}
}

func platformsFromIndex(idx v1.ImageIndex) ([]string, error) {
	im, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	for _, m := range im.Manifests {
		if m.Platform == nil {
			continue
		}
		if m.Platform.OS == "unknown" && m.Platform.Architecture == "unknown" {
			continue
		}
		if m.Platform.OS != "" && m.Platform.OS != "linux" {
			continue
		}
		a := m.Platform.Architecture
		if a == "" || a == "unknown" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	sort.Strings(out)
	return out, nil
}

const manifestAcceptFallback = "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json,application/vnd.docker.distribution.manifest.v1+json"

func parseArchitecturesFromManifestJSON(body []byte) ([]string, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if sv, ok := root["schemaVersion"].(float64); ok && sv == 1 {
		if a, ok := root["architecture"].(string); ok && a != "" && a != "unknown" {
			return []string{a}, nil
		}
	}
	manifests, ok := root["manifests"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("manifest 中无 manifests 数组")
	}
	seen := map[string]struct{}{}
	var out []string
	for _, m := range manifests {
		mm, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		p, ok := mm["platform"].(map[string]interface{})
		if !ok {
			continue
		}
		osStr, _ := p["os"].(string)
		arch, _ := p["architecture"].(string)
		if osStr == "unknown" && arch == "unknown" {
			continue
		}
		if osStr != "" && osStr != "linux" {
			continue
		}
		if arch == "" || arch == "unknown" {
			continue
		}
		if _, ok := seen[arch]; ok {
			continue
		}
		seen[arch] = struct{}{}
		out = append(out, arch)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("manifest 中未解析到有效架构")
	}
	sort.Strings(out)
	return out, nil
}

func detectArchitecturesHTTPFallback(ctx context.Context, ref name.Reference, acrRegistryID uint) ([]string, error) {
	reg := ref.Context().RegistryStr()
	repo := ref.Context().RepositoryStr()
	tag := ref.Identifier()
	u := fmt.Sprintf("https://%s/v2/%s/manifests/%s", reg, repo, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", manifestAcceptFallback)
	if auth, ok := getACRAuthByRegistryID(acrRegistryID).(*authn.Basic); ok && auth != nil && auth.Username != "" {
		req.SetBasicAuth(auth.Username, auth.Password)
	}
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseArchitecturesFromManifestJSON(body)
}

// DetectImageArchitecturesInRegistry lists linux architectures present in the registry for the given tag
// (manifest list / OCI index). Falls back to a single architecture from image config when the tag is a single-platform image.
// acrRegistryID selects which ACR credentials to use; 0 falls back to system default config.
func DetectImageArchitecturesInRegistry(imageRef string, acrRegistryID uint) ([]string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	opts := remoteOptionsForACR(ctx, acrRegistryID)

	if idx, err := remote.Index(ref, opts...); err == nil {
		arches, perr := platformsFromIndex(idx)
		if perr != nil {
			return nil, perr
		}
		if len(arches) > 0 {
			return arches, nil
		}
	}

	if img, err := remote.Image(ref, opts...); err == nil {
		cfg, cfgErr := img.ConfigFile()
		if cfgErr == nil && cfg != nil {
			a := strings.TrimSpace(cfg.Architecture)
			if a != "" && a != "unknown" {
				return []string{a}, nil
			}
		}
	}

	if arches, err := detectArchitecturesHTTPFallback(ctx, ref, acrRegistryID); err == nil && len(arches) > 0 {
		return arches, nil
	}

	return nil, fmt.Errorf("无法解析镜像 manifest 中的架构信息: %s", imageRef)
}

// ArchitecturesToJSON returns a JSON array string for DB storage.
func ArchitecturesToJSON(arches []string) string {
	if len(arches) == 0 {
		return "[]"
	}
	b, err := json.Marshal(arches)
	if err != nil {
		return "[]"
	}
	return string(b)
}
