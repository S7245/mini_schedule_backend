package persistence

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/instructor"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type instructorRepository struct {
	db *gorm.DB
}

// NewInstructorRepository 创建教练档案仓储。
func NewInstructorRepository(db *gorm.DB) instructor.Repository {
	return &instructorRepository{db: db}
}

func (r *instructorRepository) GetByBrandUserID(ctx context.Context, brandID, brandUserID int64) (*instructor.Profile, error) {
	var m InstructorProfileModel
	if err := r.db.WithContext(ctx).
		Where("brand_id = ? AND brand_user_id = ?", brandID, brandUserID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrInstructorProfileNotFound, "教练档案不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询教练档案失败", err)
	}
	return toInstructorDomain(&m), nil
}

// Upsert 1:1 校验：同 brand_user_id 已有时 update，没有时 insert。
// 整段在事务里完成（含 audit.Write）。
func (r *instructorRepository) Upsert(ctx context.Context, actorID int64, in instructor.UpsertInput) (*instructor.Profile, error) {
	var saved InstructorProfileModel

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 校验 brand_user 存在且属于本 brand
		var bu BrandUserModel
		if err := tx.Where("id = ? AND brand_id = ? AND deleted_at IS NULL", in.BrandUserID, in.BrandID).
			First(&bu).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
			}
			return apperr.ErrInternalF("查询员工失败", err)
		}

		var existing InstructorProfileModel
		err := tx.Where("brand_user_id = ?", in.BrandUserID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.ErrInternalF("查询教练档案失败", err)
		}

		action := "instructor_profile_upserted"
		var before *InstructorProfileModel
		if err == nil {
			beforeCopy := existing
			before = &beforeCopy
			saved = existing
			saved.DisplayName = in.DisplayName
			saved.AvatarURL = in.AvatarURL
			saved.Bio = in.Bio
			saved.Specialties = in.Specialties
			saved.Certificates = in.Certificates
			saved.IsVisibleToLearners = in.IsVisibleToLearners
			saved.IsSchedulable = in.IsSchedulable
			saved.Status = string(in.Status)
			if err := tx.Save(&saved).Error; err != nil {
				return apperr.ErrInternalF("更新教练档案失败", err)
			}
		} else {
			saved = InstructorProfileModel{
				BrandID:             in.BrandID,
				BrandUserID:         in.BrandUserID,
				DisplayName:         in.DisplayName,
				AvatarURL:           in.AvatarURL,
				Bio:                 in.Bio,
				Specialties:         in.Specialties,
				Certificates:        in.Certificates,
				IsVisibleToLearners: in.IsVisibleToLearners,
				IsSchedulable:       in.IsSchedulable,
				Status:              string(in.Status),
			}
			if err := tx.Create(&saved).Error; err != nil {
				if isUniqueViolation(err) {
					// 走到这里通常意味着外部并发 insert；保持幂等返回 1:1 冲突
					return apperr.NewAppError(apperr.ErrInstructorProfileNotFound, "教练档案冲突", 409)
				}
				return apperr.ErrInternalF("创建教练档案失败", err)
			}
		}

		bID := in.BrandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  action,
			Target:  audit.Target{Type: "instructor_profile", ID: saved.ID},
			Before:  before,
			After:   &saved,
		})
	})
	if err != nil {
		return nil, err
	}
	return toInstructorDomain(&saved), nil
}

func (r *instructorRepository) Delete(ctx context.Context, brandID, actorID, brandUserID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before InstructorProfileModel
		if err := tx.Where("brand_id = ? AND brand_user_id = ?", brandID, brandUserID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrInstructorProfileNotFound, "教练档案不存在", 404)
			}
			return apperr.ErrInternalF("查询教练档案失败", err)
		}
		if err := tx.Delete(&InstructorProfileModel{}, before.ID).Error; err != nil {
			return apperr.ErrInternalF("删除教练档案失败", err)
		}
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "instructor_profile_deleted",
			Target:  audit.Target{Type: "instructor_profile", ID: before.ID},
			Before:  &before,
		})
	})
}

func toInstructorDomain(m *InstructorProfileModel) *instructor.Profile {
	return &instructor.Profile{
		ID:                  m.ID,
		BrandID:             m.BrandID,
		BrandUserID:         m.BrandUserID,
		DisplayName:         m.DisplayName,
		AvatarURL:           m.AvatarURL,
		Bio:                 m.Bio,
		Specialties:         m.Specialties,
		Certificates:        m.Certificates,
		IsVisibleToLearners: m.IsVisibleToLearners,
		IsSchedulable:       m.IsSchedulable,
		Status:              instructor.Status(m.Status),
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
	}
}
