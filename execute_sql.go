package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"docker-image-sync-platform/internal/config"
	"docker-image-sync-platform/internal/database"
)

func main() {
	// 加载配置
	if err := config.LoadConfig("config.yaml"); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化数据库连接
	if err := database.InitDatabase(); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	// 读取SQL文件
	sqlFile := "sql/backfill_data.sql"
	if len(os.Args) > 1 {
		sqlFile = os.Args[1]
	}

	sqlContent, err := ioutil.ReadFile(sqlFile)
	if err != nil {
		log.Fatalf("读取SQL文件失败: %v", err)
	}

	fmt.Printf("正在执行SQL脚本: %s\n", sqlFile)
	fmt.Println("SQL内容长度:", len(sqlContent), "字节")

	// 分步执行SQL语句
	fmt.Println("开始补全历史数据...")

	// 1. 补全 original_input 字段
	fmt.Println("1. 补全 original_input 字段...")
	result1 := database.DB.Exec(`
		UPDATE image_sync_records 
		SET original_input = CASE 
			WHEN tag IS NOT NULL AND tag != '' THEN CONCAT(original_image, ':', tag)
			ELSE original_image
		END,
		updated_at = CURRENT_TIMESTAMP
		WHERE original_input IS NULL OR original_input = ''
	`)
	if result1.Error != nil {
		log.Fatalf("补全 original_input 字段失败: %v", result1.Error)
	}
	fmt.Printf("   补全 original_input 字段完成，影响行数: %d\n", result1.RowsAffected)

	// 2. 补全 started_at 字段
	fmt.Println("2. 补全 started_at 字段...")
	result2 := database.DB.Exec(`
		UPDATE image_sync_records 
		SET started_at = created_at,
			updated_at = CURRENT_TIMESTAMP
		WHERE started_at IS NULL
	`)
	if result2.Error != nil {
		log.Fatalf("补全 started_at 字段失败: %v", result2.Error)
	}
	fmt.Printf("   补全 started_at 字段完成，影响行数: %d\n", result2.RowsAffected)

	// 3. 补全 completed_at 字段
	fmt.Println("3. 补全 completed_at 字段...")
	result3 := database.DB.Exec(`
		UPDATE image_sync_records 
		SET completed_at = updated_at
		WHERE completed_at IS NULL 
		AND sync_status IN ('success', 'failed')
	`)
	if result3.Error != nil {
		log.Fatalf("补全 completed_at 字段失败: %v", result3.Error)
	}
	fmt.Printf("   补全 completed_at 字段完成，影响行数: %d\n", result3.RowsAffected)

	// 4. 计算并更新 duration 字段
	fmt.Println("4. 计算并更新 duration 字段...")
	result4 := database.DB.Exec(`
		UPDATE image_sync_records 
		SET duration = TIMESTAMPDIFF(SECOND, started_at, completed_at),
			updated_at = CURRENT_TIMESTAMP
		WHERE started_at IS NOT NULL 
		AND completed_at IS NOT NULL 
		AND (duration IS NULL OR duration = 0)
	`)
	if result4.Error != nil {
		log.Fatalf("计算 duration 字段失败: %v", result4.Error)
	}
	fmt.Printf("   计算 duration 字段完成，影响行数: %d\n", result4.RowsAffected)

	// 5. 验证补全结果
	fmt.Println("5. 验证补全结果...")
	var stats struct {
		TotalRecords      int64 `gorm:"column:total_records"`
		HasOriginalInput  int64 `gorm:"column:has_original_input"`
		HasStartedAt      int64 `gorm:"column:has_started_at"`
		HasCompletedAt    int64 `gorm:"column:has_completed_at"`
		HasDuration       int64 `gorm:"column:has_duration"`
	}
	
	database.DB.Raw(`
		SELECT
			COUNT(*) as total_records,
			SUM(CASE WHEN original_input IS NOT NULL AND original_input != '' THEN 1 ELSE 0 END) as has_original_input,
			SUM(CASE WHEN started_at IS NOT NULL THEN 1 ELSE 0 END) as has_started_at,
			SUM(CASE WHEN completed_at IS NOT NULL THEN 1 ELSE 0 END) as has_completed_at,
			SUM(CASE WHEN duration IS NOT NULL AND duration > 0 THEN 1 ELSE 0 END) as has_duration
		FROM image_sync_records
	`).Scan(&stats)

	fmt.Printf("验证结果:\n")
	fmt.Printf("   总记录数: %d\n", stats.TotalRecords)
	fmt.Printf("   有 original_input 的记录: %d\n", stats.HasOriginalInput)
	fmt.Printf("   有 started_at 的记录: %d\n", stats.HasStartedAt)
	fmt.Printf("   有 completed_at 的记录: %d\n", stats.HasCompletedAt)
	fmt.Printf("   有 duration 的记录: %d\n", stats.HasDuration)

	fmt.Println("数据补全完成！")
}