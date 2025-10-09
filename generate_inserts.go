package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// parseImageInfo 解析镜像信息
func parseImageInfo(imageStr string) (image, tag, architecture string) {
	// 处理架构信息
	if strings.Contains(imageStr, "--platform=") {
		parts := strings.Fields(imageStr)
		for i, part := range parts {
			if strings.HasPrefix(part, "--platform=") {
				architecture = strings.TrimPrefix(part, "--platform=")
				// 移除架构参数，重新组合镜像字符串
				parts = append(parts[:i], parts[i+1:]...)
				imageStr = strings.Join(parts, " ")
				break
			}
		}
	}

	// 解析镜像和标签
	if strings.Contains(imageStr, ":") {
		parts := strings.Split(imageStr, ":")
		image = parts[0]
		tag = parts[1]
	} else {
		image = imageStr
		tag = "latest"
	}

	return strings.TrimSpace(image), strings.TrimSpace(tag), strings.TrimSpace(architecture)
}

// generateACRImage 生成阿里云ACR镜像地址
func generateACRImage(originalImage, tag string) string {
	return generateACRImageWithArchitecture(originalImage, tag, "")
}

// generateACRImageWithArchitecture 生成带架构信息的阿里云ACR镜像地址
func generateACRImageWithArchitecture(originalImage, tag, architecture string) string {
	registry := "registry.cn-hangzhou.aliyuncs.com"
	namespace := "lpx03" // 根据示例使用 lpx03

	// 解析镜像名称
	imageName := originalImage
	if strings.Contains(imageName, "/") {
		parts := strings.Split(imageName, "/")
		imageName = parts[len(parts)-1]
	}
	
	// 生成架构后缀，将架构信息添加到tag后面
	architectureSuffix := ""
	if architecture != "" && architecture != "amd64" {
		// 将简化的架构名转换为完整的平台字符串
		var platform string
		switch architecture {
		case "arm64":
			platform = "linux/arm64"
		case "arm":
			platform = "linux/arm"
		case "386":
			platform = "linux/386"
		default:
			// 如果已经是完整格式（如linux/arm64），直接使用
			if strings.Contains(architecture, "/") {
				platform = architecture
			} else {
				platform = "linux/" + architecture
			}
		}
		// 将 linux/arm64 转换为 -linux-arm64
		architectureSuffix = "-" + strings.ReplaceAll(platform, "/", "-")
	}
	
	// 构建最终的镜像名称和标签
	finalTag := tag
	if finalTag == "" {
		finalTag = "latest"
	}
	
	// 构建标签：tag + architectureSuffix
	finalTagWithArch := finalTag + architectureSuffix
	
	return fmt.Sprintf("%s/%s/%s:%s", registry, namespace, imageName, finalTagWithArch)
}

func main() {
	// 读取 images.txt 文件
	file, err := os.Open("images.txt")
	if err != nil {
		fmt.Printf("Error opening images.txt: %v\n", err)
		return
	}
	defer file.Close()

	// 创建输出文件
	outputFile, err := os.Create("images_insert.sql")
	if err != nil {
		fmt.Printf("Error creating images_insert.sql: %v\n", err)
		return
	}
	defer outputFile.Close()

	// 写入文件头部注释
	outputFile.WriteString("-- MySQL INSERT statements for image_sync_records\n")
	outputFile.WriteString("-- Generated at: " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")

	scanner := bufio.NewScanner(file)
	id := 1

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // 跳过空行和注释行
		}

		// 解析镜像信息
		originalImage, tag, architecture := parseImageInfo(line)
		
		// 生成 ACR 地址
		acrImage := generateACRImageWithArchitecture(originalImage, tag, architecture)
		
		// 生成 UUID
		taskID := uuid.New().String()
		
		// 生成当前时间
		now := time.Now().Format("2006-01-02 15:04:05.000")
		
		// 生成 INSERT 语句
		insertSQL := fmt.Sprintf(
			"INSERT INTO `docker_sync`.`image_sync_records` (`original_image`, `acr_image`, `tag`, `architecture`, `sync_status`, `error_message`, `task_id`, `created_at`, `updated_at`, `deleted_at`) VALUES ('%s', '%s', '%s', '%s', 'success', '', '%s', '%s', '%s', NULL);\n",
			originalImage, acrImage, tag, architecture, taskID, now, now,
		)
		
		outputFile.WriteString(insertSQL)
		id++
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading images.txt: %v\n", err)
		return
	}

	fmt.Printf("Successfully generated %d INSERT statements in images_insert.sql\n", id-1)
}