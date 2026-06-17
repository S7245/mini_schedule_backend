package persistence

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/domain/learner"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

func ptrI64(v int64) *int64 { return &v }

// seedActiveSubscription 给 brand 造一条 active 订阅（含 saas_plan），供 SubscriptionGuard 通过。
func seedActiveSubscription(t *testing.T, db *gorm.DB, brandID int64, maxLearners int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO saas_plans (name, max_locations, max_staff_seats, max_learners) VALUES (?, 100, 100, ?)`,
		"测试套餐", maxLearners,
	).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	var planID int64
	if err := db.Raw(`SELECT id FROM saas_plans ORDER BY id DESC LIMIT 1`).Scan(&planID).Error; err != nil {
		t.Fatalf("read plan id: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO brand_subscriptions (brand_id, plan_id, billing_cycle, status, starts_at, expires_at, max_locations, max_staff_seats, max_learners)
		 VALUES (?, ?, 'monthly', 'active', NOW() - INTERVAL '1 day', NOW() + INTERVAL '30 days', 100, 100, ?)`,
		brandID, planID, maxLearners,
	).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
}

func newLearnerRepo(db *gorm.DB) learner.Repository {
	return NewLearnerRepository(db, &commercial.SubscriptionGuard{})
}

func TestLearner_CreateHappyWithDenormAndTags(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)
	loc := seedLocation(t, db, brandID, "讯美广场")

	tag, err := repo.CreateTag(context.Background(), learner.CreateTagInput{BrandID: brandID, ActorID: 1, Name: "VIP", Color: "#f00"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}

	p, err := repo.Create(context.Background(), learner.CreateInput{
		BrandID: brandID, ActorID: 1, Phone: "13700001111", Nickname: "学员A",
		PrimaryLocationID: ptrI64(loc), LearnerNo: "S001", Remark: "备注", TagIDs: []int64{tag.ID},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Status != learner.StatusActive {
		t.Fatalf("status = %s, want active", p.Status)
	}
	if p.Phone != "13700001111" {
		t.Fatalf("denorm phone = %q", p.Phone)
	}
	if p.PrimaryLocationName != "讯美广场" {
		t.Fatalf("denorm primary_location_name = %q", p.PrimaryLocationName)
	}
	if p.LearnerNo != "S001" {
		t.Fatalf("learner_no = %q", p.LearnerNo)
	}
	if len(p.Tags) != 1 || p.Tags[0].Name != "VIP" {
		t.Fatalf("tags = %+v, want [VIP]", p.Tags)
	}
	// identity 已建（合成 open_id）。
	var openID string
	if err := db.Raw(`SELECT wechat_open_id FROM learner_identities WHERE phone = ?`, "13700001111").Scan(&openID).Error; err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if openID != "manual:13700001111" {
		t.Fatalf("synthetic open_id = %q", openID)
	}
}

func TestLearner_CreateDuplicatePhoneSameBrand(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)

	in := learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13700002222", Nickname: "甲"}
	if _, err := repo.Create(context.Background(), in); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.Create(context.Background(), in)
	assertAppCode(t, err, apperr.ErrLearnerAlreadyExists)
}

func TestLearner_CreateDuplicateLearnerNo(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)

	if _, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13700003333", LearnerNo: "X1"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13700004444", LearnerNo: "X1"})
	assertAppCode(t, err, apperr.ErrLearnerNoDuplicated)
}

func TestLearner_CreateInvalidTagRollsBack(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)

	_, err := repo.Create(context.Background(), learner.CreateInput{
		BrandID: brandID, ActorID: 1, Phone: "13700005555", TagIDs: []int64{999999},
	})
	assertAppCode(t, err, apperr.ErrLearnerTagNotFound)
	// tx 回滚：profile 未创建。
	var cnt int64
	db.Raw(`SELECT COUNT(*) FROM brand_learner_profiles WHERE brand_id = ?`, brandID).Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("profile count = %d, want 0 (rolled back)", cnt)
	}
}

func TestLearner_CreateQuotaExceeded(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 1) // max_learners = 1

	if _, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13700006666"}); err != nil {
		t.Fatalf("first within quota: %v", err)
	}
	_, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13700007777"})
	assertAppCode(t, err, apperr.ErrQuotaExceeded)
}

func TestLearner_IdentityReusedAcrossBrands(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandA, _ := seedBrandWithSystemRoles(t, db)
	brandB, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandA, 100)
	seedActiveSubscription(t, db, brandB, 100)

	pa, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandA, ActorID: 1, Phone: "13700008888", Nickname: "甲"})
	if err != nil {
		t.Fatalf("brandA create: %v", err)
	}
	pb, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandB, ActorID: 1, Phone: "13700008888", Nickname: "乙"})
	if err != nil {
		t.Fatalf("brandB create: %v", err)
	}
	if pa.LearnerIdentityID != pb.LearnerIdentityID {
		t.Fatalf("identity not reused: A=%d B=%d", pa.LearnerIdentityID, pb.LearnerIdentityID)
	}
}

func TestLearner_ListFiltersAndScope(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)
	loc1 := seedLocation(t, db, brandID, "门店1")
	loc2 := seedLocation(t, db, brandID, "门店2")

	mustCreate := func(phone, nick string, loc int64) {
		if _, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: phone, Nickname: nick, PrimaryLocationID: ptrI64(loc)}); err != nil {
			t.Fatalf("create %s: %v", phone, err)
		}
	}
	mustCreate("13710000001", "张三", loc1)
	mustCreate("13710000002", "李四", loc2)

	// q 搜索 phone 子串。
	items, total, err := repo.List(context.Background(), learner.ListFilter{BrandID: brandID, Query: "0000001"}, 0, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].Phone != "13710000001" {
		t.Fatalf("q search → total=%d items=%v err=%v", total, items, err)
	}
	// 主门店过滤。
	_, total, err = repo.List(context.Background(), learner.ListFilter{BrandID: brandID, PrimaryLocationID: loc2}, 0, 20)
	if err != nil || total != 1 {
		t.Fatalf("location filter → total=%d err=%v", total, err)
	}
	// data_scope：仅 loc1。
	_, total, err = repo.List(context.Background(), learner.ListFilter{BrandID: brandID, ScopeLocationIDs: []int64{loc1}}, 0, 20)
	if err != nil || total != 1 {
		t.Fatalf("scope loc1 → total=%d err=%v", total, err)
	}
	// data_scope 空集 → 0。
	_, total, err = repo.List(context.Background(), learner.ListFilter{BrandID: brandID, ScopeLocationIDs: []int64{}}, 0, 20)
	if err != nil || total != 0 {
		t.Fatalf("scope empty → total=%d err=%v", total, err)
	}
}

func TestLearner_UpdateAndStatus(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)
	tag, _ := repo.CreateTag(context.Background(), learner.CreateTagInput{BrandID: brandID, ActorID: 1, Name: "标签1"})

	p, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13720000001", Nickname: "原名"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	newNick := "新名"
	tags := []int64{tag.ID}
	got, err := repo.Update(context.Background(), brandID, 1, p.ID, learner.UpdateInput{Nickname: &newNick, TagIDs: &tags})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Nickname != "新名" || len(got.Tags) != 1 {
		t.Fatalf("update result nick=%q tags=%v", got.Nickname, got.Tags)
	}
	// freeze。
	got, err = repo.UpdateStatus(context.Background(), brandID, 1, p.ID, string(learner.StatusFrozen))
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if got.Status != learner.StatusFrozen {
		t.Fatalf("status = %s, want frozen", got.Status)
	}
}

func TestLearner_DeleteSoftAndReferenceGuard(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	seedActiveSubscription(t, db, brandID, 100)

	p, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13730000001"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 软删成功 → 列表不再可见。
	if err := repo.Delete(context.Background(), brandID, 1, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), brandID, p.ID); apperr.GetAppError(err) == nil || apperr.GetAppError(err).Code != apperr.ErrLearnerNotFound {
		t.Fatalf("get after delete → want LEARNER_NOT_FOUND, got %v", err)
	}

	// 引用保护：有 active 权益 → LEARNER_IN_USE。
	p2, err := repo.Create(context.Background(), learner.CreateInput{BrandID: brandID, ActorID: 1, Phone: "13730000002"})
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}
	seedActiveEntitlement(t, db, brandID, p2.ID)
	err = repo.Delete(context.Background(), brandID, 1, p2.ID)
	assertAppCode(t, err, apperr.ErrLearnerInUse)
}

// seedActiveEntitlement 给学员造一条 active 权益（含 entitlement_product），供 IN_USE guard 测试。
func seedActiveEntitlement(t *testing.T, db *gorm.DB, brandID, profileID int64) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO entitlement_products (brand_id, name, product_type, total_credits, validity_days) VALUES (?, '次卡', 'class_pack', 10, 30)`,
		brandID,
	).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	var pid int64
	db.Raw(`SELECT id FROM entitlement_products WHERE brand_id = ? ORDER BY id DESC LIMIT 1`, brandID).Scan(&pid)
	if err := db.Exec(
		`INSERT INTO learner_entitlements (brand_id, brand_learner_profile_id, product_id, status, starts_at, expires_at)
		 VALUES (?, ?, ?, 'active', NOW(), NOW() + INTERVAL '30 days')`,
		brandID, profileID, pid,
	).Error; err != nil {
		t.Fatalf("seed entitlement: %v", err)
	}
}

func TestLearnerTag_CRUDAndDuplicate(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newLearnerRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	tag, err := repo.CreateTag(context.Background(), learner.CreateTagInput{BrandID: brandID, ActorID: 1, Name: "新客", Color: "#0f0"})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	// 重名。
	_, err = repo.CreateTag(context.Background(), learner.CreateTagInput{BrandID: brandID, ActorID: 1, Name: "新客"})
	assertAppCode(t, err, apperr.ErrLearnerTagNameDuplicated)

	// 改名 + 停用。
	newName := "老客"
	inactive := string(learner.TagStatusInactive)
	got, err := repo.UpdateTag(context.Background(), brandID, 1, tag.ID, learner.UpdateTagInput{Name: &newName, Status: &inactive})
	if err != nil {
		t.Fatalf("update tag: %v", err)
	}
	if got.Name != "老客" || got.Status != learner.TagStatusInactive {
		t.Fatalf("update tag result name=%q status=%s", got.Name, got.Status)
	}
	// 按 status 过滤列表。
	tags, err := repo.ListTags(context.Background(), learner.TagListFilter{BrandID: brandID, Status: string(learner.TagStatusActive)})
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("active tags = %d, want 0 (the only tag is inactive)", len(tags))
	}
}
