-- =============================================================================
-- 回填 image_sync_records / sync_tasks 的 acr_registry_id
-- 依据 acr_image 前缀匹配 acr_registries.registry_url + namespace
--
-- acr_image 示例:
--   registry.cn-hangzhou.aliyuncs.com/lpx03/nginx:1.10.3
--   crpi-xxx.cn-hangzhou.personal.cr.aliyuncs.com/lpx0312/nginx:1.27.0
--
-- 使用前请先备份数据库，并在测试环境验证。
-- =============================================================================

USE docker_sync;

-- -----------------------------------------------------------------------------
-- 1. 预览：当前 ACR 配置
-- -----------------------------------------------------------------------------
SELECT id, registry_url, namespace, is_default
FROM acr_registries
WHERE deleted_at IS NULL
ORDER BY id;

-- -----------------------------------------------------------------------------
-- 2. 预览：待回填的同步记录（acr_registry_id 为空/0 且有 acr_image）
-- -----------------------------------------------------------------------------
SELECT
  r.id,
  r.acr_registry_id,
  r.acr_image,
  ar.id   AS matched_registry_id,
  ar.registry_url,
  ar.namespace
FROM image_sync_records r
LEFT JOIN acr_registries ar
  ON ar.deleted_at IS NULL
 AND (
      r.acr_image LIKE CONCAT(ar.registry_url, '/', ar.namespace, '/%')
   OR REPLACE(REPLACE(r.acr_image, 'https://', ''), 'http://', '')
      LIKE CONCAT(ar.registry_url, '/', ar.namespace, '/%')
 )
WHERE r.deleted_at IS NULL
  AND r.acr_image IS NOT NULL
  AND r.acr_image <> ''
  AND (r.acr_registry_id IS NULL OR r.acr_registry_id = 0)
ORDER BY r.id, ar.id;

-- -----------------------------------------------------------------------------
-- 3. 回填 image_sync_records.acr_registry_id
--    匹配规则: acr_image 以 {registry_url}/{namespace}/ 开头
--    若多条 ACR 同时命中（极少见），优先选 registry_url 更长的一条
--    兼容 MySQL 5.7（不使用窗口函数）
-- -----------------------------------------------------------------------------
START TRANSACTION;

UPDATE image_sync_records r
INNER JOIN (
  SELECT r2.id AS record_id, MIN(ar.id) AS registry_id
  FROM image_sync_records r2
  INNER JOIN acr_registries ar
    ON ar.deleted_at IS NULL
   AND (
        r2.acr_image LIKE CONCAT(ar.registry_url, '/', ar.namespace, '/%')
     OR REPLACE(REPLACE(r2.acr_image, 'https://', ''), 'http://', '')
        LIKE CONCAT(ar.registry_url, '/', ar.namespace, '/%')
   )
  INNER JOIN (
    SELECT
      r3.id AS record_id,
      MAX(LENGTH(ar3.registry_url)) AS max_url_len
    FROM image_sync_records r3
    INNER JOIN acr_registries ar3
      ON ar3.deleted_at IS NULL
     AND (
          r3.acr_image LIKE CONCAT(ar3.registry_url, '/', ar3.namespace, '/%')
       OR REPLACE(REPLACE(r3.acr_image, 'https://', ''), 'http://', '')
          LIKE CONCAT(ar3.registry_url, '/', ar3.namespace, '/%')
     )
    WHERE r3.deleted_at IS NULL
      AND r3.acr_image IS NOT NULL
      AND r3.acr_image <> ''
      AND (r3.acr_registry_id IS NULL OR r3.acr_registry_id = 0)
    GROUP BY r3.id
  ) pick ON pick.record_id = r2.id
        AND LENGTH(ar.registry_url) = pick.max_url_len
  WHERE r2.deleted_at IS NULL
    AND r2.acr_image IS NOT NULL
    AND r2.acr_image <> ''
    AND (r2.acr_registry_id IS NULL OR r2.acr_registry_id = 0)
  GROUP BY r2.id
) m ON r.id = m.record_id
SET r.acr_registry_id = m.registry_id,
    r.updated_at = NOW();

-- 查看本次影响行数
SELECT ROW_COUNT() AS image_sync_records_updated;

COMMIT;

-- -----------------------------------------------------------------------------
-- 4. 回填 sync_tasks.acr_registry_id（从同 task_id 的同步记录推断）
--    仅更新仍为 0/NULL 的任务；同一 task 下若存在多个 ACR，取出现次数最多的
--    兼容 MySQL 5.7（不使用窗口函数）
-- -----------------------------------------------------------------------------
START TRANSACTION;

UPDATE sync_tasks st
INNER JOIN (
  SELECT rc.task_id, MIN(rc.acr_registry_id) AS acr_registry_id
  FROM (
    SELECT task_id, acr_registry_id, COUNT(*) AS cnt
    FROM image_sync_records
    WHERE deleted_at IS NULL
      AND task_id IS NOT NULL
      AND task_id <> ''
      AND acr_registry_id IS NOT NULL
      AND acr_registry_id > 0
    GROUP BY task_id, acr_registry_id
  ) rc
  INNER JOIN (
    SELECT task_id, MAX(cnt) AS max_cnt
    FROM (
      SELECT task_id, acr_registry_id, COUNT(*) AS cnt
      FROM image_sync_records
      WHERE deleted_at IS NULL
        AND task_id IS NOT NULL
        AND task_id <> ''
        AND acr_registry_id IS NOT NULL
        AND acr_registry_id > 0
      GROUP BY task_id, acr_registry_id
    ) t
    GROUP BY task_id
  ) best ON best.task_id = rc.task_id AND best.max_cnt = rc.cnt
  GROUP BY rc.task_id
) m ON st.task_id = m.task_id
SET st.acr_registry_id = m.acr_registry_id,
    st.updated_at = NOW()
WHERE st.deleted_at IS NULL
  AND (st.acr_registry_id IS NULL OR st.acr_registry_id = 0);

SELECT ROW_COUNT() AS sync_tasks_updated;

COMMIT;

-- -----------------------------------------------------------------------------
-- 5. 验证：仍未能匹配的记录（需人工处理）
-- -----------------------------------------------------------------------------
SELECT
  id,
  task_id,
  acr_registry_id,
  acr_image,
  original_image,
  sync_status,
  created_at
FROM image_sync_records
WHERE deleted_at IS NULL
  AND acr_image IS NOT NULL
  AND acr_image <> ''
  AND (acr_registry_id IS NULL OR acr_registry_id = 0)
ORDER BY id;

-- -----------------------------------------------------------------------------
-- 6. 验证：按 ACR 统计回填结果
-- -----------------------------------------------------------------------------
SELECT
  COALESCE(ar.namespace, '(未关联)') AS acr_namespace,
  r.acr_registry_id,
  COUNT(*) AS record_count
FROM image_sync_records r
LEFT JOIN acr_registries ar ON ar.id = r.acr_registry_id AND ar.deleted_at IS NULL
WHERE r.deleted_at IS NULL
GROUP BY r.acr_registry_id, ar.namespace
ORDER BY record_count DESC;
