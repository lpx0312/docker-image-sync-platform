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

// getACRAuth returns an authenticator for the configured ACR instance, or anonymous if unset.
func getACRAuth() authn.Authenticator {
	user := systemConfigValue("aliyun_username")
	rawPass := systemConfigValue("aliyun_password")
	if user == "" {
		return authn.Anonymous
	}
	pass, err := DecryptSystemConfigValue(rawPass)
	if err != nil {
		logger.Logger.Warn("解密阿里云密码失败，将尝试匿名访问", zap.Error(err))
		pass = rawPass
	}
	if pass == "" {
		return authn.Anonymous
	}
	return &authn.Basic{Username: user, Password: pass}
}

func remoteOptions(ctx context.Context) []remote.Option {
	return []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuth(getACRAuth()),
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

func detectArchitecturesHTTPFallback(ctx context.Context, ref name.Reference) ([]string, error) {
	reg := ref.Context().RegistryStr()
	repo := ref.Context().RepositoryStr()
	tag := ref.Identifier()
	u := fmt.Sprintf("https://%s/v2/%s/manifests/%s", reg, repo, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", manifestAcceptFallback)
	user := systemConfigValue("aliyun_username")
	rawPass := systemConfigValue("aliyun_password")
	if user != "" {
		pass, derr := DecryptSystemConfigValue(rawPass)
		if derr != nil {
			pass = rawPass
		}
		if pass != "" {
			req.SetBasicAuth(user, pass)
		}
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
func DetectImageArchitecturesInRegistry(imageRef string) ([]string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	opts := remoteOptions(ctx)

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

	if arches, err := detectArchitecturesHTTPFallback(ctx, ref); err == nil && len(arches) > 0 {
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
