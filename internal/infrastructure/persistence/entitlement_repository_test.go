package persistence

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

func seedLearnerProfile(t *testing.T, db *gorm.DB, brandID int64, openID string) int64 {
	t.Helper()
	if err := db.Exec(`INSERT INTO learner_identities (wechat_open_id, status) VALUES (?, 'active')`, openID).Error; err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	var iid int64
	db.Raw(`SELECT id FROM learner_identities WHERE wechat_open_id = ?`, openID).Scan(&iid)
	if err := db.Exec(`INSERT INTO brand_learner_profiles (brand_id, learner_identity_id, status) VALUES (?, ?, 'active')`, brandID, iid).Error; err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	var pid int64
	db.Raw(`SELECT id FROM brand_learner_profiles WHERE brand_id = ? AND learner_identity_id = ?`, brandID, iid).Scan(&pid)
	return pid
}

func seedCourse(t *testing.T, db *gorm.DB, brandID int64, title string) int64 {
	t.Helper()
	if err := db.Exec(`INSERT INTO courses (brand_id, title, duration_min, status) VALUES (?, ?, 60, 'published')`, brandID, title).Error; err != nil {
		t.Fatalf("seed course: %v", err)
	}
	var id int64
	db.Raw(`SELECT id FROM courses WHERE brand_id = ? AND title = ? ORDER BY id DESC LIMIT 1`, brandID, title).Scan(&id)
	return id
}

func newEntitlementRepo(db *gorm.DB) entitlement.Repository { return NewEntitlementRepository(db) }

// entGrantSetup 造 brand + 一个真实 brand_user 作 actor（granted_by/operated_by 有 FK）+ 一个学员。
func entGrantSetup(t *testing.T, db *gorm.DB) (brandID, actor, learner int64) {
	t.Helper()
	brandID, _ = seedBrandWithSystemRoles(t, db)
	actor = seedBrandUser(t, db, brandID)
	learner = seedLearnerProfile(t, db, brandID, "ent:learner")
	return
}

func classPackInput(brandID int64, name string) entitlement.CreateProductInput {
	return entitlement.CreateProductInput{
		BrandID: brandID, ActorID: 1, Name: name, ProductType: "class_pack",
		TotalCredits: 10, ValidityDays: 90, LocationScope: "all", CourseScope: "all",
	}
}

func membershipInput(brandID int64, name string) entitlement.CreateProductInput {
	return entitlement.CreateProductInput{
		BrandID: brandID, ActorID: 1, Name: name, ProductType: "membership_card",
		ValidityDays: 30, LocationScope: "all", CourseScope: "all",
	}
}

// ---- 产品 ----

func TestEntitlementProduct_CreateClassPackAndGet(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	p, err := repo.CreateProduct(context.Background(), classPackInput(brandID, "10次卡"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.TotalCredits == nil || *p.TotalCredits != 10 {
		t.Fatalf("total_credits = %v, want 10", p.TotalCredits)
	}
	if p.Status != entitlement.ProductStatusActive || p.IssuedCount != 0 {
		t.Fatalf("status=%s issued=%d", p.Status, p.IssuedCount)
	}
	if len(p.LocationIDs) != 0 || len(p.CourseIDs) != 0 {
		t.Fatalf("scope=all should have empty ids, got loc=%v course=%v", p.LocationIDs, p.CourseIDs)
	}
}

func TestEntitlementProduct_CreateMembershipUnlimited(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	in := membershipInput(brandID, "月卡")
	in.DailyBookingLimit = 1
	p, err := repo.CreateProduct(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.TotalCredits != nil {
		t.Fatalf("membership total_credits should be nil, got %v", *p.TotalCredits)
	}
	if p.DailyBookingLimit == nil || *p.DailyBookingLimit != 1 {
		t.Fatalf("daily limit = %v, want 1", p.DailyBookingLimit)
	}
}

func TestEntitlementProduct_CreateSpecificScope(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")
	course := seedCourse(t, db, brandID, "瑜伽")

	in := classPackInput(brandID, "专用卡")
	in.LocationScope, in.CourseScope = "specific", "specific"
	in.LocationIDs, in.CourseIDs = []int64{loc}, []int64{course}
	p, err := repo.CreateProduct(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(p.LocationIDs) != 1 || p.LocationIDs[0] != loc {
		t.Fatalf("location_ids = %v, want [%d]", p.LocationIDs, loc)
	}
	if len(p.CourseIDs) != 1 || p.CourseIDs[0] != course {
		t.Fatalf("course_ids = %v, want [%d]", p.CourseIDs, course)
	}
}

func TestEntitlementProduct_ScopeInvalid(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	in := classPackInput(brandID, "坏范围卡")
	in.LocationScope = "specific"
	in.LocationIDs = []int64{999999}
	_, err := repo.CreateProduct(context.Background(), in)
	assertAppCode(t, err, apperr.ErrEntitlementScopeInvalid)
}

func TestEntitlementProduct_DuplicateActiveName(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)

	if _, err := repo.CreateProduct(context.Background(), classPackInput(brandID, "同名")); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := repo.CreateProduct(context.Background(), classPackInput(brandID, "同名"))
	assertAppCode(t, err, apperr.ErrEntitlementProductNameDuplicated)
}

func TestEntitlementProduct_UpdateAndStatus(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, _ := seedBrandWithSystemRoles(t, db)
	loc := seedLocation(t, db, brandID, "门店1")

	p, _ := repo.CreateProduct(context.Background(), classPackInput(brandID, "卡A"))
	newName := "卡A改"
	weekly := 3
	specific := "specific"
	got, err := repo.UpdateProduct(context.Background(), brandID, 1, p.ID, entitlement.UpdateProductInput{
		Name: &newName, WeeklyBookingLimit: &weekly, LocationScope: &specific, LocationIDs: &[]int64{loc},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "卡A改" || got.WeeklyBookingLimit == nil || *got.WeeklyBookingLimit != 3 || len(got.LocationIDs) != 1 {
		t.Fatalf("update result name=%q weekly=%v locs=%v", got.Name, got.WeeklyBookingLimit, got.LocationIDs)
	}
	got, err = repo.UpdateProductStatus(context.Background(), brandID, 1, p.ID, "inactive")
	if err != nil || got.Status != entitlement.ProductStatusInactive {
		t.Fatalf("disable → status=%s err=%v", got.Status, err)
	}
}

// ---- 学员权益 ----

func TestEntitlement_GrantPackWithLedger(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), classPackInput(brandID, "10次卡"))

	e, err := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if e.RemainingCredits == nil || *e.RemainingCredits != 10 || e.Status != entitlement.StatusActive {
		t.Fatalf("granted remaining=%v status=%s", e.RemainingCredits, e.Status)
	}
	txns, err := repo.ListTransactions(context.Background(), brandID, e.ID)
	if err != nil || len(txns) != 1 || txns[0].Action != entitlement.ActionGrant || txns[0].DeltaCredits != 10 {
		t.Fatalf("ledger = %+v err=%v", txns, err)
	}
	if txns[0].BalanceAfter == nil || *txns[0].BalanceAfter != 10 {
		t.Fatalf("grant balance_after = %v, want 10", txns[0].BalanceAfter)
	}
	gp, _ := repo.GetProduct(context.Background(), brandID, prod.ID)
	if gp.IssuedCount != 1 {
		t.Fatalf("issued_count = %d, want 1", gp.IssuedCount)
	}
}

func TestEntitlement_GrantMembershipUnlimited(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), membershipInput(brandID, "月卡"))

	e, err := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if e.RemainingCredits != nil || e.TotalCredits != nil || e.Status != entitlement.StatusActive {
		t.Fatalf("membership grant remaining=%v total=%v status=%s", e.RemainingCredits, e.TotalCredits, e.Status)
	}
}

func TestEntitlement_GrantFromInactiveProduct(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), classPackInput(brandID, "停用卡"))
	repo.UpdateProductStatus(context.Background(), brandID, 1, prod.ID, "inactive")

	_, err := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})
	assertAppCode(t, err, apperr.ErrEntitlementProductInactive)
}

func TestEntitlement_GrantToMissingLearner(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, _ := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), classPackInput(brandID, "卡"))

	_, err := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: 999999, ProductID: prod.ID})
	assertAppCode(t, err, apperr.ErrLearnerNotFound)
}

func TestEntitlement_AdjustAndInsufficient(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), classPackInput(brandID, "卡"))
	e, _ := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})

	e2, err := repo.Adjust(context.Background(), entitlement.AdjustInput{BrandID: brandID, ActorID: actor, EntitlementID: e.ID, Delta: 5, Reason: "补偿"})
	if err != nil || *e2.RemainingCredits != 15 {
		t.Fatalf("adjust +5 → remaining=%v err=%v", e2.RemainingCredits, err)
	}
	e3, err := repo.Adjust(context.Background(), entitlement.AdjustInput{BrandID: brandID, ActorID: actor, EntitlementID: e.ID, Delta: -3, Reason: "扣"})
	if err != nil || *e3.RemainingCredits != 12 {
		t.Fatalf("adjust -3 → remaining=%v err=%v", e3.RemainingCredits, err)
	}
	_, err = repo.Adjust(context.Background(), entitlement.AdjustInput{BrandID: brandID, ActorID: actor, EntitlementID: e.ID, Delta: -999, Reason: "x"})
	assertAppCode(t, err, apperr.ErrEntitlementInsufficient)
	cur, _ := repo.GetEntitlement(context.Background(), brandID, e.ID)
	if *cur.RemainingCredits != 12 {
		t.Fatalf("after rejected adjust remaining=%v, want 12", cur.RemainingCredits)
	}
	txns, _ := repo.ListTransactions(context.Background(), brandID, e.ID)
	if len(txns) != 3 {
		t.Fatalf("ledger len = %d, want 3 (grant + 2 adjust)", len(txns))
	}
}

func TestEntitlement_AdjustUnlimitedRejected(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), membershipInput(brandID, "月卡"))
	e, _ := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})
	_, err := repo.Adjust(context.Background(), entitlement.AdjustInput{BrandID: brandID, ActorID: actor, EntitlementID: e.ID, Delta: 1, Reason: "x"})
	assertAppCode(t, err, apperr.ErrEntitlementInsufficient)
}

func TestEntitlement_SettleDepletedOnAdjust(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	in := classPackInput(brandID, "5次卡")
	in.TotalCredits = 5
	prod, _ := repo.CreateProduct(context.Background(), in)
	e, _ := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})
	got, err := repo.Adjust(context.Background(), entitlement.AdjustInput{BrandID: brandID, ActorID: actor, EntitlementID: e.ID, Delta: -5, Reason: "清零"})
	if err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if got.Status != entitlement.StatusDepleted {
		t.Fatalf("status after remaining=0 = %s, want depleted", got.Status)
	}
}

func TestEntitlement_SettleExpiredOnList(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), classPackInput(brandID, "卡"))
	e, err := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	// starts+expires 一起挪到过去（满足 expires>starts 的 CHECK），模拟过期。
	res := db.Exec(`UPDATE learner_entitlements SET starts_at = now() - interval '10 days', expires_at = now() - interval '1 day' WHERE id = ?`, e.ID)
	if res.Error != nil {
		t.Fatalf("force expire: %v", res.Error)
	}
	if res.RowsAffected != 1 {
		t.Fatalf("force expire matched %d rows, want 1 (entitlement id=%d)", res.RowsAffected, e.ID)
	}
	if _, err := repo.ListEntitlementsByLearner(context.Background(), brandID, learner); err != nil {
		t.Fatalf("list: %v", err)
	}
	var status string
	db.Raw(`SELECT status FROM learner_entitlements WHERE id = ?`, e.ID).Scan(&status)
	if status != "expired" {
		t.Fatalf("DB status after sweep = %q, want expired", status)
	}
}

func TestEntitlement_StatusFreezeReactivateCancel(t *testing.T) {
	db := newMigratedTestDB(t)
	repo := newEntitlementRepo(db)
	brandID, actor, learner := entGrantSetup(t, db)
	prod, _ := repo.CreateProduct(context.Background(), classPackInput(brandID, "卡"))
	e, _ := repo.Grant(context.Background(), entitlement.GrantInput{BrandID: brandID, ActorID: actor, LearnerID: learner, ProductID: prod.ID})

	got, err := repo.SetEntitlementStatus(context.Background(), brandID, actor, e.ID, "frozen", "暂停")
	if err != nil || got.Status != entitlement.StatusFrozen {
		t.Fatalf("freeze → %s err=%v", got.Status, err)
	}
	got, err = repo.SetEntitlementStatus(context.Background(), brandID, actor, e.ID, "active", "恢复")
	if err != nil || got.Status != entitlement.StatusActive {
		t.Fatalf("reactivate → %s err=%v", got.Status, err)
	}
	got, err = repo.SetEntitlementStatus(context.Background(), brandID, actor, e.ID, "cancelled", "作废")
	if err != nil || got.Status != entitlement.StatusCancelled {
		t.Fatalf("cancel → %s err=%v", got.Status, err)
	}
	_, err = repo.Adjust(context.Background(), entitlement.AdjustInput{BrandID: brandID, ActorID: actor, EntitlementID: e.ID, Delta: 1, Reason: "x"})
	assertAppCode(t, err, apperr.ErrEntitlementNotAdjustable)
	_, err = repo.SetEntitlementStatus(context.Background(), brandID, actor, e.ID, "active", "x")
	assertAppCode(t, err, apperr.ErrEntitlementNotAdjustable)
}
