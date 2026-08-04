package store

import (
	"github.com/momaek/tolato/server/internal/model"
	"gorm.io/gorm"
)

// --- User groups ---

func CreateUserGroup(g *model.UserGroup) error { return DB.Create(g).Error }

func GetUserGroupByID(id string) (*model.UserGroup, error) {
	var g model.UserGroup
	if err := DB.First(&g, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func ListUserGroups() ([]model.UserGroup, error) {
	var gs []model.UserGroup
	err := DB.Order("name ASC").Find(&gs).Error
	return gs, err
}

func UpdateUserGroup(id string, updates map[string]any) error {
	return DB.Model(&model.UserGroup{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteUserGroup removes the group along with its memberships and any grants
// naming it as the subject — otherwise those grants would keep matching a
// group id that no longer resolves to anyone.
func DeleteUserGroup(id string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_group_id = ?", id).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("subject_type = ? AND subject_id = ?", model.SubjectUserGroup, id).
			Delete(&model.Grant{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.UserGroup{}).Error
	})
}

// SetUserGroupMembers replaces the group's membership with exactly userIDs.
func SetUserGroupMembers(groupID string, userIDs []string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_group_id = ?", groupID).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		if len(userIDs) == 0 {
			return nil
		}
		rows := make([]model.UserGroupMember, 0, len(userIDs))
		for _, uid := range userIDs {
			rows = append(rows, model.UserGroupMember{UserGroupID: groupID, UserID: uid})
		}
		return tx.Create(&rows).Error
	})
}

// ListUserGroupMemberIDs returns the user ids in one group.
func ListUserGroupMemberIDs(groupID string) ([]string, error) {
	var ids []string
	err := DB.Model(&model.UserGroupMember{}).Where("user_group_id = ?", groupID).
		Pluck("user_id", &ids).Error
	return ids, err
}

// ListGroupIDsForUser returns the groups a user belongs to.
func ListGroupIDsForUser(userID string) ([]string, error) {
	var ids []string
	err := DB.Model(&model.UserGroupMember{}).Where("user_id = ?", userID).
		Pluck("user_group_id", &ids).Error
	return ids, err
}

// RemoveUserFromAllGroups clears a deleted user's memberships.
func RemoveUserFromAllGroups(tx *gorm.DB, userID string) error {
	return tx.Where("user_id = ?", userID).Delete(&model.UserGroupMember{}).Error
}

// --- Node groups ---

func CreateNodeGroup(g *model.NodeGroup) error { return DB.Create(g).Error }

func GetNodeGroupByID(id string) (*model.NodeGroup, error) {
	var g model.NodeGroup
	if err := DB.First(&g, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func ListNodeGroups() ([]model.NodeGroup, error) {
	var gs []model.NodeGroup
	err := DB.Order("name ASC").Find(&gs).Error
	return gs, err
}

func UpdateNodeGroup(id string, updates map[string]any) error {
	return DB.Model(&model.NodeGroup{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteNodeGroup removes the group, its memberships, the grants pointing at
// it, and its use as a registration token's target group.
func DeleteNodeGroup(id string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_group_id = ?", id).Delete(&model.NodeGroupMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("object_type = ? AND object_id = ?", model.ObjectNodeGroup, id).
			Delete(&model.Grant{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.RegistrationToken{}).Where("node_group_id = ?", id).
			Update("node_group_id", nil).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.NodeGroup{}).Error
	})
}

// SetNodeGroupMembers replaces the group's membership with exactly nodeIDs.
func SetNodeGroupMembers(groupID string, nodeIDs []string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_group_id = ?", groupID).Delete(&model.NodeGroupMember{}).Error; err != nil {
			return err
		}
		if len(nodeIDs) == 0 {
			return nil
		}
		rows := make([]model.NodeGroupMember, 0, len(nodeIDs))
		for _, nid := range nodeIDs {
			rows = append(rows, model.NodeGroupMember{NodeGroupID: groupID, NodeID: nid})
		}
		return tx.Create(&rows).Error
	})
}

// AddNodeToGroup enrols one node, ignoring a repeat enrolment. Used on agent
// registration, where the same token may bring up many machines.
func AddNodeToGroup(groupID, nodeID string) error {
	return DB.Where(model.NodeGroupMember{NodeGroupID: groupID, NodeID: nodeID}).
		FirstOrCreate(&model.NodeGroupMember{NodeGroupID: groupID, NodeID: nodeID}).Error
}

// ListNodeGroupMemberIDs returns the node ids in one group.
func ListNodeGroupMemberIDs(groupID string) ([]string, error) {
	var ids []string
	err := DB.Model(&model.NodeGroupMember{}).Where("node_group_id = ?", groupID).
		Pluck("node_id", &ids).Error
	return ids, err
}

// ListGroupIDsForNode returns the groups a node belongs to.
func ListGroupIDsForNode(nodeID string) ([]string, error) {
	var ids []string
	err := DB.Model(&model.NodeGroupMember{}).Where("node_id = ?", nodeID).
		Pluck("node_group_id", &ids).Error
	return ids, err
}

// GroupNamesByNode maps node id → group names, for listing views that want to
// show membership without one query per row.
func GroupNamesByNode() (map[string][]string, error) {
	type row struct {
		NodeID string
		Name   string
	}
	var rows []row
	err := DB.Table("node_group_members AS m").
		Select("m.node_id AS node_id, g.name AS name").
		Joins("JOIN node_groups AS g ON g.id = m.node_group_id").
		Order("g.name ASC").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(rows))
	for _, r := range rows {
		out[r.NodeID] = append(out[r.NodeID], r.Name)
	}
	return out, nil
}

// GroupIDsByNode maps node id → group ids in a single query, for permission
// evaluation over a whole listing.
func GroupIDsByNode() (map[string][]string, error) {
	var rows []model.NodeGroupMember
	if err := DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(rows))
	for _, r := range rows {
		out[r.NodeID] = append(out[r.NodeID], r.NodeGroupID)
	}
	return out, nil
}

