package services

// acr_services_test.go：acr_affinity / acr_registry / acr_repository 三个服务。
// 远程校验分支用不可达地址（连接拒绝 → check_failed）；成功路径的远程交互
// 由 generic_api_test / acr_api_test 覆盖。

import (
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"

	"docker-image-sync-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// svcEnc 固定密钥加密服务
func svcEnc(t *testing.T) *EncryptionService {
	t.Helper()
	t.Setenv("ENCRYPTION_KEY", "svc-acr-test-key")
	t.Setenv("GIN_MODE", "debug")
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)
	enc, err := NewEncryptionService(l)
	require.NoError(t, err)
	return enc
}

// svcAcr 直插一条仓库配置（type 默认 acr，密码加密）
func svcAcr(t *testing.T, db *gorm.DB, enc *EncryptionService, alias, ns string, isDefault bool, regType string) *models.AcrRegistry {
	t.Helper()
	if regType == "" {
		regType = models.RegistryTypeACR
	}
	pwd, err := enc.Encrypt("pw-" + alias)
	require.NoError(t, err)
	acr := &models.AcrRegistry{
		RegistryURL: "127.0.0.1:1", Namespace: ns, Alias: alias,
		Username: "u", Password: pwd, RegistryType: regType, IsDefault: isDefault,
	}
	require.NoError(t, db.Create(acr).Error)
	return acr
}

func svcRec(t *testing.T, db *gorm.DB, acrID uint, image, status string) *models.ImageSyncRecord {
	t.Helper()
	rec := &models.ImageSyncRecord{
		AcrRegistryID: acrID, OriginalImage: image, SyncStatus: status, Tag: "1",
	}
	require.NoError(t, db.Create(rec).Error)
	return rec
}

// ====================================================================
// AcrRegistryService
// ====================================================================

func TestAcrRegistryService_All(t *testing.T) {
	db := newSvcTestDB(t)
	enc := svcEnc(t)
	svc := NewAcrRegistryService(db, enc)

	t.Run("GetAll_空与列表", func(t *testing.T) {
		list, err := svc.GetAll()
		require.NoError(t, err)
		assert.Empty(t, list)
		svcAcr(t, db, enc, "a1", "ns1", true, "")
		list, err = svc.GetAll()
		require.NoError(t, err)
		assert.Len(t, list, 1)
	})

	t.Run("GetByID_存在与不存在", func(t *testing.T) {
		got, err := svc.GetByID(1)
		require.NoError(t, err)
		assert.Equal(t, "a1", got.Alias)

		_, err = svc.GetByID(999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不存在")
	})

	t.Run("GetDefault_未设置与命中", func(t *testing.T) {
		got, err := svc.GetDefault()
		require.NoError(t, err)
		assert.Equal(t, "a1", got.Alias)

		// 软删唯一默认后未设置默认
		require.NoError(t, db.Delete(&models.AcrRegistry{}, 1).Error)
		_, err = svc.GetDefault()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "未设置默认ACR")
	})

	t.Run("Create_类型规范化与别名兜底", func(t *testing.T) {
		created, err := svc.Create(&models.AcrRegistryRequest{
			RegistryURL: "reg.example.com", Namespace: "ns2",
			Username: "u", Password: "p", RegistryType: " SWR ",
		})
		require.NoError(t, err)
		assert.Equal(t, models.RegistryTypeSWR, created.RegistryType)
		assert.Equal(t, "ns2", created.Alias, "空别名默认取 namespace")

		_, err = svc.Create(&models.AcrRegistryRequest{
			RegistryURL: "r", Namespace: "", Alias: " ",
			Username: "u", Password: "p",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "别名不能为空")
	})

	t.Run("Create_空类型默认acr_首个自动默认", func(t *testing.T) {
		db2 := newSvcTestDB(t)
		svc2 := NewAcrRegistryService(db2, enc)
		created, err := svc2.Create(&models.AcrRegistryRequest{
			RegistryURL: "r", Namespace: "n", Username: "u", Password: "p",
		})
		require.NoError(t, err)
		assert.Equal(t, models.RegistryTypeACR, created.RegistryType)
		assert.True(t, created.IsDefault)
	})

	t.Run("Update_别名/密码/类型/AK-SK矩阵", func(t *testing.T) {
		updated, err := svc.Update(2, &models.AcrRegistryUpdateRequest{
			Alias: "renamed", RegistryURL: "new.example.com", Namespace: "ns2b",
			Username: "u2", Password: "newpw", RegistryType: "harbor",
			AccessKey: "ak-1", SecretKey: "sk-1", AuthServer: "auth.example.com", DockerService: "svc",
		})
		require.NoError(t, err)
		assert.Equal(t, "renamed", updated.Alias)
		assert.Equal(t, "new.example.com", updated.RegistryURL)
		assert.Equal(t, models.RegistryTypeHarbor, updated.RegistryType)
		assert.Equal(t, "ak-1", updated.AccessKey)
		dec, err := enc.Decrypt(updated.Password)
		require.NoError(t, err)
		assert.Equal(t, "newpw", dec)
		decSK, err := enc.Decrypt(updated.SecretKey)
		require.NoError(t, err)
		assert.Equal(t, "sk-1", decSK)

		// 占位 *** 不更新密码；空字段不动
		before := updated.Password
		updated2, err := svc.Update(2, &models.AcrRegistryUpdateRequest{Password: "***", SecretKey: "***"})
		require.NoError(t, err)
		assert.Equal(t, before, updated2.Password)
	})

	t.Run("Update_别名冲突与非法类型", func(t *testing.T) {
		svcAcr(t, db, enc, "occupied", "ns9", false, "")
		_, err := svc.Update(2, &models.AcrRegistryUpdateRequest{Alias: "occupied"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "已被其他镜像仓库使用")

		_, err = svc.Update(2, &models.AcrRegistryUpdateRequest{RegistryType: "bogus"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不支持的仓库类型")

		_, err = svc.Update(999, &models.AcrRegistryUpdateRequest{})
		require.Error(t, err)
	})

	t.Run("Update_命名空间变更时别名跟随", func(t *testing.T) {
		db3 := newSvcTestDB(t)
		svc3 := NewAcrRegistryService(db3, enc)
		created, err := svc3.Create(&models.AcrRegistryRequest{
			RegistryURL: "r", Namespace: "old-ns", Username: "u", Password: "p",
		})
		require.NoError(t, err)
		assert.Equal(t, "old-ns", created.Alias)

		updated, err := svc3.Update(created.ID, &models.AcrRegistryUpdateRequest{Namespace: "new-ns"})
		require.NoError(t, err)
		assert.Equal(t, "new-ns", updated.Alias, "别名==旧命名空间时应跟随命名空间更新")
	})

	t.Run("Delete_默认转移与唯一限制", func(t *testing.T) {
		db4 := newSvcTestDB(t)
		svc4 := NewAcrRegistryService(db4, enc)
		first := svcAcr(t, db4, enc, "d1", "n1", true, "")
		second := svcAcr(t, db4, enc, "d2", "n2", false, "")

		require.NoError(t, svc4.Delete(first.ID))
		var promoted models.AcrRegistry
		require.NoError(t, db4.Where("id = ?", second.ID).First(&promoted).Error)
		assert.True(t, promoted.IsDefault, "删除默认后应转移默认")

		// second 已是默认且唯一 → 不能删
		err := svc4.Delete(second.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "唯一")

		// 追加非默认后：非默认可删；默认仍受保护
		third := svcAcr(t, db4, enc, "d3", "n3", false, "")
		require.NoError(t, svc4.Delete(third.ID))
		require.Error(t, svc4.Delete(second.ID))

		require.Error(t, svc4.Delete(999))
	})

	t.Run("SetDefault_事务切换", func(t *testing.T) {
		db5 := newSvcTestDB(t)
		svc5 := NewAcrRegistryService(db5, enc)
		a := svcAcr(t, db5, enc, "s1", "n1", true, "")
		b := svcAcr(t, db5, enc, "s2", "n2", false, "")
		require.NoError(t, svc5.SetDefault(b.ID))
		// 每次查询用新变量：复用带主键的 dest 会把旧 ID 拼进 WHERE
		var gotA models.AcrRegistry
		require.NoError(t, db5.First(&gotA, a.ID).Error)
		assert.False(t, gotA.IsDefault)
		var gotB models.AcrRegistry
		require.NoError(t, db5.First(&gotB, b.ID).Error)
		assert.True(t, gotB.IsDefault)

		require.Error(t, svc5.SetDefault(999))
	})

	t.Run("TestConnection_不存在与解密失败", func(t *testing.T) {
		_, err := svc.TestConnection(999)
		require.Error(t, err)

		bad := &models.AcrRegistry{RegistryURL: "127.0.0.1:1", Namespace: "n", Alias: "badenc", Username: "u", Password: "ENC:!!!", RegistryType: models.RegistryTypeACR}
		require.NoError(t, db.Create(bad).Error)
		_, err = svc.TestConnection(bad.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解密")
	})

	// 不做 ACR 类型的连通性成功/失败断言：客户端会对 dockerauth.*.aliyuncs.com
	// 发起真实外网请求（匿名 scope 也能取到 token），离线单测无法确定性覆盖
}

// ====================================================================
// AcrRepositoryService
// ====================================================================

func TestAcrRepositoryService_All(t *testing.T) {
	db := newSvcTestDB(t)
	enc := svcEnc(t)
	svc := NewAcrRepositoryService(db, enc)
	acr := svcAcr(t, db, enc, "r1", "ns1", true, "")

	t.Run("GetAll_排序与GetByID", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "zeta"}).Error)
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "alpha"}).Error)
		list, err := svc.GetAll(acr.ID)
		require.NoError(t, err)
		require.Len(t, list, 2)
		assert.Equal(t, "alpha", list[0].RepositoryName)

		got, err := svc.GetByID(list[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "alpha", got.RepositoryName)
		_, err = svc.GetByID(999)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不存在")
	})

	t.Run("Create_配置不存在与已存在", func(t *testing.T) {
		_, err := svc.Create(&models.AcrRepositoryRequest{AcrRegistryID: 999, RepositoryName: "x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "配置不存在")

		res, err := svc.Create(&models.AcrRepositoryRequest{AcrRegistryID: acr.ID, RepositoryName: "alpha"})
		require.NoError(t, err)
		assert.False(t, res.Created)
		assert.Equal(t, "already_exist", res.Reason)
	})

	t.Run("Create_解密失败与检查失败", func(t *testing.T) {
		bad := &models.AcrRegistry{RegistryURL: "127.0.0.1:1", Namespace: "n", Alias: "badenc2", Username: "u", Password: "ENC:%%%", RegistryType: models.RegistryTypeACR}
		require.NoError(t, db.Create(bad).Error)
		_, err := svc.Create(&models.AcrRepositoryRequest{AcrRegistryID: bad.ID, RepositoryName: "x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解密")

		// 不可达 → 非NotFound错误 → check_failed
		res, err := svc.Create(&models.AcrRepositoryRequest{AcrRegistryID: acr.ID, RepositoryName: "remote-fail"})
		require.NoError(t, err)
		assert.Equal(t, "check_failed", res.Reason)
	})

	t.Run("EnsureRepository_幂等与空参", func(t *testing.T) {
		require.NoError(t, svc.EnsureRepository(acr.ID, " ensured "))
		var count int64
		db.Model(&models.AcrRepository{}).Where("acr_registry_id = ? AND repository_name = ?", acr.ID, "ensured").Count(&count)
		assert.EqualValues(t, 1, count)
		require.NoError(t, svc.EnsureRepository(acr.ID, "ensured"))
		db.Model(&models.AcrRepository{}).Where("repository_name = ?", "ensured").Count(&count)
		assert.EqualValues(t, 1, count, "重复 Ensure 不应新增")

		require.NoError(t, svc.EnsureRepository(0, "x"))
		require.NoError(t, svc.EnsureRepository(acr.ID, ""))
	})

	t.Run("Delete与BatchDelete", func(t *testing.T) {
		rec := &models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "del"}
		require.NoError(t, db.Create(rec).Error)
		require.NoError(t, svc.Delete(rec.ID))
		// 再次删除已软删记录：GORM 不报错（RowsAffected=0）
		require.NoError(t, svc.Delete(rec.ID))

		rec2 := &models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: "bd"}
		require.NoError(t, db.Create(rec2).Error)
		n, err := svc.BatchDelete([]uint{rec2.ID, 999})
		require.NoError(t, err)
		assert.Equal(t, 1, n)
	})

	t.Run("CleanInvalid_配置不存在与解密失败", func(t *testing.T) {
		_, err := svc.CleanInvalid(999)
		require.Error(t, err)
		bad := &models.AcrRegistry{RegistryURL: "127.0.0.1:1", Namespace: "n", Alias: "badenc3", Username: "u", Password: "ENC:###", RegistryType: models.RegistryTypeACR}
		require.NoError(t, db.Create(bad).Error)
		_, err = svc.CleanInvalid(bad.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解密")
	})

	t.Run("CleanInvalid_不可达全部检查失败", func(t *testing.T) {
		target := svcAcr(t, db, enc, "clean", "ns2", false, "")
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: target.ID, RepositoryName: "one"}).Error)
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: target.ID, RepositoryName: "two"}).Error)
		result, err := svc.CleanInvalid(target.ID)
		require.NoError(t, err)
		assert.Equal(t, 0, result.Cleaned)
		assert.Len(t, result.CheckFailedNames, 2)
		var count int64
		db.Model(&models.AcrRepository{}).Where("acr_registry_id = ?", target.ID).Count(&count)
		assert.EqualValues(t, 2, count, "检查失败不应删除")
	})

	t.Run("SyncFromRecords_已有与检查失败与新建前校验", func(t *testing.T) {
		target := svcAcr(t, db, enc, "sync", "ns3", false, "")
		// 已在台账 → already_exist
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: target.ID, RepositoryName: "exists"}).Error)
		svcRec(t, db, target.ID, "registry.example.com/ns3/exists:v1", models.SyncStatusSuccess)
		// 远程不可达 → check_failed
		svcRec(t, db, target.ID, "registry.example.com/ns3/newimg:v1", models.SyncStatusSuccess)
		// 失败记录不参与
		svcRec(t, db, target.ID, "registry.example.com/ns3/failedimg:v1", models.SyncStatusFailed)

		result, err := svc.SyncFromRecords(target.ID)
		require.NoError(t, err)
		assert.Equal(t, 1, result.AlreadyExist)
		assert.Contains(t, result.AlreadyExistNames, "exists")
		assert.Equal(t, 1, result.Skipped)
		assert.Contains(t, result.CheckFailedNames, "newimg")
		assert.Equal(t, 0, result.Created)

		_, err = svc.SyncFromRecords(999)
		require.Error(t, err)
	})

	t.Run("ImportFromRegistry_错误与空列表", func(t *testing.T) {
		_, err := svc.ImportFromRegistry(999)
		require.Error(t, err)

		bad := &models.AcrRegistry{RegistryURL: "127.0.0.1:1", Namespace: "n", Alias: "badenc4", Username: "u", Password: "ENC:$$$", RegistryType: models.RegistryTypeACR}
		require.NoError(t, db.Create(bad).Error)
		_, err = svc.ImportFromRegistry(bad.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "解密")

		// 不可达 → ListRepositories 报错（ACR 客户端 _catalog）
		_, err = svc.ImportFromRegistry(acr.ID)
		require.Error(t, err)
	})
}

// ====================================================================
// AcrAffinityService
// ====================================================================

func TestAcrAffinityService_All(t *testing.T) {
	db := newSvcTestDB(t)
	enc := svcEnc(t)
	svc := NewAcrAffinityService(db)

	acrA := svcAcr(t, db, enc, "ali-a", "ns-a", true, "")                       // 默认，acr（300 配额）
	acrB := svcAcr(t, db, enc, "ali-b", "ns-b", false, models.RegistryTypeSWR) // SWR 不限配额

	t.Run("FindAffinity_空名与无归属", func(t *testing.T) {
		res, err := svc.FindAffinity("")
		require.NoError(t, err)
		assert.False(t, res.HasAffinity)
		assert.Equal(t, "none", res.Source)

		res, err = svc.FindAffinity("ghost")
		require.NoError(t, err)
		assert.False(t, res.HasAffinity)
	})

	t.Run("FindAffinity_台账归属优先", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acrA.ID, RepositoryName: "nginx"}).Error)
		// 同名成功记录指向 B —— 台账(A)优先
		svcRec(t, db, acrB.ID, "reg/ns-b/nginx:1", models.SyncStatusSuccess)

		res, err := svc.FindAffinity("library/nginx:1.25")
		require.NoError(t, err)
		assert.True(t, res.HasAffinity)
		assert.Equal(t, acrA.ID, res.AcrRegistryID)
		assert.Equal(t, "repository", res.Source)
		assert.Equal(t, DefaultAcrRepoQuota, res.RepoQuota)
		// 成功记录归属 B，A 下 Tag 数为 0
		assert.Equal(t, 0, res.TagCount)
	})

	t.Run("FindAffinity_同步记录归属兜底", func(t *testing.T) {
		svcRec(t, db, acrB.ID, "reg/ns-b/redis:1", models.SyncStatusSuccess)
		res, err := svc.FindAffinity("redis")
		require.NoError(t, err)
		assert.True(t, res.HasAffinity)
		assert.Equal(t, acrB.ID, res.AcrRegistryID)
		assert.Equal(t, "sync_record", res.Source)
		assert.Equal(t, 0, res.RepoQuota, "SWR 不限配额")
	})

	t.Run("FindAffinity_台账指向已删除ACR_报错", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: 999, RepositoryName: "orphan"}).Error)
		_, err := svc.FindAffinity("orphan")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "获取 ACR 配置失败")
	})

	t.Run("SuggestAcrForImage", func(t *testing.T) {
		res, err := svc.SuggestAcrForImage("nginx")
		require.NoError(t, err)
		assert.True(t, res.HasAffinity)
	})

	t.Run("ResolveTargetAcr_归属优先", func(t *testing.T) {
		res, err := svc.ResolveTargetAcr("nginx")
		require.NoError(t, err)
		assert.Equal(t, "affinity", res.SuggestionReason)
		assert.Equal(t, acrA.ID, res.SuggestedAcrID)
	})

	t.Run("ResolveTargetAcr_默认可用", func(t *testing.T) {
		res, err := svc.ResolveTargetAcr("fresh-img")
		require.NoError(t, err)
		assert.Equal(t, "default", res.SuggestionReason)
	})

	newFullAcr := func(t *testing.T, alias string) *models.AcrRegistry {
		acr := svcAcr(t, db, enc, alias, "ns-"+alias, false, "")
		for i := 0; i < DefaultAcrRepoQuota; i++ {
			require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acr.ID, RepositoryName: fmt.Sprintf("%s-%d", alias, i)}).Error)
		}
		return acr
	}

	t.Run("ResolveTargetAcr_默认满转其他", func(t *testing.T) {
		db2 := newSvcTestDB(t)
		enc2 := svcEnc(t)
		svc2 := NewAcrAffinityService(db2)
		full := svcAcr(t, db2, enc2, "full", "ns-full", true, "")
		free := svcAcr(t, db2, enc2, "free", "ns-free", false, "")
		for i := 0; i < DefaultAcrRepoQuota; i++ {
			require.NoError(t, db2.Create(&models.AcrRepository{AcrRegistryID: full.ID, RepositoryName: fmt.Sprintf("f%d", i)}).Error)
		}
		res, err := svc2.ResolveTargetAcr("brand-new")
		require.NoError(t, err)
		assert.Equal(t, "alternate", res.SuggestionReason)
		assert.Equal(t, free.ID, res.SuggestedAcrID)
	})

	t.Run("ResolveTargetAcr_全满回退默认", func(t *testing.T) {
		db3 := newSvcTestDB(t)
		enc3 := svcEnc(t)
		svc3 := NewAcrAffinityService(db3)
		fullDefault := svcAcr(t, db3, enc3, "fd", "ns-fd", true, "")
		for i := 0; i < DefaultAcrRepoQuota; i++ {
			require.NoError(t, db3.Create(&models.AcrRepository{AcrRegistryID: fullDefault.ID, RepositoryName: fmt.Sprintf("x%d", i)}).Error)
		}
		res, err := svc3.ResolveTargetAcr("another-new")
		require.NoError(t, err)
		assert.Equal(t, "default_full", res.SuggestionReason)
		assert.Equal(t, fullDefault.ID, res.SuggestedAcrID)
	})

	t.Run("ResolveTargetAcr_无ACR_零值兜底", func(t *testing.T) {
		db4 := newSvcTestDB(t)
		svc4 := NewAcrAffinityService(db4)
		res, err := svc4.ResolveTargetAcr("any")
		require.NoError(t, err)
		assert.Equal(t, uint(0), res.SuggestedAcrID)
		assert.Empty(t, res.SuggestionReason)
	})

	t.Run("CheckConflict_各分支", func(t *testing.T) {
		// 无归属
		info, err := svc.CheckConflict("nope", acrA.ID)
		require.NoError(t, err)
		assert.False(t, info.HasConflict)
		// 有归属 + 同 ACR
		info, err = svc.CheckConflict("nginx", acrA.ID)
		require.NoError(t, err)
		assert.False(t, info.HasConflict)
		// 有归属 + 未选择(0)
		info, err = svc.CheckConflict("nginx", 0)
		require.NoError(t, err)
		assert.False(t, info.HasConflict)
		// 有归属 + 不同 ACR → 冲突
		info, err = svc.CheckConflict("nginx", acrB.ID)
		require.NoError(t, err)
		assert.True(t, info.HasConflict)
		assert.Equal(t, acrA.ID, info.SuggestedAcrID)
		assert.Contains(t, info.Message, "原仓库")
	})

	t.Run("CheckImages_冲突与多仓库告警", func(t *testing.T) {
		// nginx 归属 A；redis 归属 B → 多仓库告警
		result, err := svc.CheckImages([]string{"nginx", "redis", "brand-new-2"}, acrA.ID)
		require.NoError(t, err)
		assert.True(t, result.HasAnyConflict, "redis 选 A 与归属 B 冲突")
		assert.Equal(t, 1, result.NewRepositoryCount)
		assert.True(t, result.MultiAcrWarning)
		require.Len(t, result.Items, 3)

		// 不选仓库(0)：无冲突
		result, err = svc.CheckImages([]string{"redis"}, 0)
		require.NoError(t, err)
		assert.False(t, result.HasAnyConflict)
	})

	t.Run("GetQuotaSummary_配额计算", func(t *testing.T) {
		summary, err := svc.GetQuotaSummary()
		require.NoError(t, err)
		require.Len(t, summary, 2)
		// 默认排前
		assert.Equal(t, acrA.ID, summary[0].AcrRegistryID)
		assert.Equal(t, DefaultAcrRepoQuota, summary[0].RepoQuota)
		assert.False(t, summary[0].IsFull)
		// SWR 不限
		assert.Equal(t, 0, summary[1].RepoQuota)
		assert.False(t, summary[1].IsFull)
	})

	t.Run("GetDuplicateRepositories", func(t *testing.T) {
		dups, err := svc.GetDuplicateRepositories()
		require.NoError(t, err)
		assert.Empty(t, dups)

		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acrA.ID, RepositoryName: "shared"}).Error)
		require.NoError(t, db.Create(&models.AcrRepository{AcrRegistryID: acrB.ID, RepositoryName: "shared"}).Error)
		dups, err = svc.GetDuplicateRepositories()
		require.NoError(t, err)
		require.Len(t, dups, 1)
		assert.Equal(t, "shared", dups[0].RepositoryName)
		assert.Equal(t, 2, dups[0].AcrCount)
		require.Len(t, dups[0].Registries, 2)
	})

	_ = newFullAcr // 大数据量场景在上面独立子库中完成，避免拖慢主库
}
