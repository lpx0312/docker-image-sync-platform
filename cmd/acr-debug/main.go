package main

import (
	"fmt"
	"os"
	"strings"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/database"
	"docker-image-sync-platform/internal/logger"
	"docker-image-sync-platform/internal/models"
	"docker-image-sync-platform/internal/services"

	"github.com/sirupsen/logrus"
)

func main() {
	if err := config.LoadConfig("config.yaml"); err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}
	if err := logger.InitLogger(); err != nil {
		fmt.Fprintln(os.Stderr, "logger:", err)
		os.Exit(1)
	}
	if err := database.InitDatabase(); err != nil {
		fmt.Fprintln(os.Stderr, "db:", err)
		os.Exit(1)
	}
	defer database.CloseDatabase()

	var acr models.AcrRegistry
	if err := database.DB.First(&acr, 1).Error; err != nil {
		fmt.Fprintln(os.Stderr, "load acr:", err)
		os.Exit(1)
	}
	fmt.Printf("ACR: id=%d namespace=%s registry=%s auth=%q dockerSvc=%q\n",
		acr.ID, acr.Namespace, acr.RegistryURL, acr.AuthServer, acr.DockerService)

	enc, err := services.NewEncryptionService(logrus.New())
	if err != nil {
		fmt.Fprintln(os.Stderr, "encryption:", err)
		os.Exit(1)
	}
	pass, err := enc.Decrypt(acr.Password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decrypt:", err)
		os.Exit(1)
	}
	fmt.Printf("Decrypted password: %q (len=%d)\n", pass, len(pass))

	api := services.NewAcrAPIService()
	repos := []string{
		"docker-image-sync-platform",
		"nfs-subdir-external-provisioner",
		"longhorn-instance-manager",
		"redis",
	}

	for _, repo := range repos {
		// Step 1: GetToken
		token, err := api.GetToken(acr.RegistryURL, acr.Username, pass, acr.Namespace, repo,
			acr.AuthServer, acr.DockerService)
		if err != nil {
			fmt.Printf("  REPO %s: GetToken ERROR: %v\n", repo, err)
			continue
		}
		fmt.Printf("  REPO %s: GetToken OK, token_prefix=%q\n", repo, func() string {
			if len(token) > 8 {
				return token[:8]
			}
			return token
		}())

		// Step 2: GetTags
		tags, err := api.GetTags(acr.RegistryURL, acr.Username, pass, acr.Namespace, repo,
			acr.AuthServer, acr.DockerService)
		if err != nil {
			fmt.Printf("  REPO %s: GetTags ERROR: %v\n", repo, err)
			continue
		}
		fmt.Printf("  REPO %s: GetTags OK, tag_count=%d", repo, len(tags))
		if len(tags) > 0 {
			fmt.Printf(" first_tags=%v", tags[:min(3, len(tags))])
		}
		fmt.Println()

		// Step 3: RepositoryExists
		exists, err := api.RepositoryExists(acr.RegistryURL, acr.Username, pass, acr.Namespace, repo,
			acr.AuthServer, acr.DockerService)
		fmt.Printf("  REPO %s: RepositoryExists=%v err=%v\n", repo, exists, err)

		// Step 4: Simulate BatchCreate DB check
		var count int64
		database.DB.Model(&models.AcrRepository{}).
			Where("acr_registry_id = ? AND repository_name = ?", acr.ID, repo).
			Count(&count)
		fmt.Printf("  REPO %s: local_db_exists=%v\n", repo, count > 0)

		if !exists || count > 0 {
			continue
		}

		// Would be created
		fmt.Printf("  REPO %s: >>> WOULD BE ADDED <<<\n", repo)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
