UPDATE image_sync_records
SET input_order=0;

UPDATE image_sync_records
SET duration = 0;

UPDATE image_sync_records
SET started_at = CURRENT_TIMESTAMP;

UPDATE image_sync_records
SET completed_at = CURRENT_TIMESTAMP;

UPDATE image_sync_records
SET created_at = CURRENT_TIMESTAMP;

UPDATE image_sync_records
SET updated_at = CURRENT_TIMESTAMP;

UPDATE image_sync_records 
SET original_input = CASE 
    -- 如果架构是arm64，则添加--platform=linux/arm64前缀
    WHEN architecture = 'arm64' THEN 
        CASE 
            WHEN tag IS NOT NULL AND tag != '' THEN CONCAT('--platform=linux/arm64 ', original_image, ':', tag)
            ELSE CONCAT('--platform=linux/arm64 ', original_image)
        END
    -- 其他架构（主要是amd64）直接使用镜像名
    ELSE 
        CASE 
            WHEN tag IS NOT NULL AND tag != '' THEN CONCAT(original_image, ':', tag)
            ELSE original_image
        END
END,
updated_at = CURRENT_TIMESTAMP
WHERE original_input IS NULL OR original_input = '';