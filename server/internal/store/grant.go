package store

import (
	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// UpsertGrant creates the grant, or updates the level if the same subject is
// already granted on the same object. The unique index makes "grant again with
// a different level" an edit rather than a duplicate row.
func UpsertGrant(g *model.Grant) error {
	return upsertGrant(DB, g)
}

// UpsertGrants applies several grants as one unit, so granting a subject five
// node groups either lands whole or not at all — a half-applied set of rules is
// worse than none, because it looks deliberate.
func UpsertGrants(gs []*model.Grant) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, g := range gs {
			if err := upsertGrant(tx, g); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertGrant(db *gorm.DB, g *model.Grant) error {
	var existing model.Grant
	err := db.Where("subject_type = ? AND subject_id = ? AND object_type = ? AND object_id = ?",
		g.SubjectType, g.SubjectID, g.ObjectType, g.ObjectID).First(&existing).Error
	switch {
	case err == nil:
		g.ID = existing.ID
		g.CreatedAt = existing.CreatedAt
		return db.Model(&model.Grant{}).Where("id = ?", existing.ID).
			Updates(map[string]any{"level": g.Level, "created_by": g.CreatedBy}).Error
	case err == gorm.ErrRecordNotFound:
		return db.Create(g).Error
	default:
		return err
	}
}

func ListGrants() ([]model.Grant, error) {
	var gs []model.Grant
	err := DB.Order("created_at DESC").Find(&gs).Error
	return gs, err
}

func DeleteGrant(id string) error {
	return DB.Where("id = ?", id).Delete(&model.Grant{}).Error
}

// DeleteGrantsForSubject drops every grant held by a user or group, called when
// that subject is deleted so no rule outlives the thing it named.
func DeleteGrantsForSubject(tx *gorm.DB, subjectType, subjectID string) error {
	return tx.Where("subject_type = ? AND subject_id = ?", subjectType, subjectID).
		Delete(&model.Grant{}).Error
}

// GrantsForSubjects returns every grant held by the given user and their
// groups. One query, so evaluating a request's permissions is a single round
// trip rather than one per object.
func GrantsForSubjects(userID string, groupIDs []string) ([]model.Grant, error) {
	q := DB.Where("subject_type = ? AND subject_id = ?", model.SubjectUser, userID)
	if len(groupIDs) > 0 {
		q = q.Or("subject_type = ? AND subject_id IN ?", model.SubjectUserGroup, groupIDs)
	}
	var gs []model.Grant
	err := DB.Where(q).Find(&gs).Error
	return gs, err
}
