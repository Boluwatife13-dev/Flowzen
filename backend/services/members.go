// Package services holds the business rules shared by several handlers.
package services

import "flowzen/models"

// MemberRole returns the user's role in a member list, or "" if they are not in it.
func MemberRole(members []models.Member, id models.ID) string {
	for _, m := range members {
		if m.UserID == id {
			return m.Role
		}
	}
	return ""
}

func WithoutMember(members []models.Member, id models.ID) []models.Member {
	kept := []models.Member{}
	for _, m := range members {
		if m.UserID != id {
			kept = append(kept, m)
		}
	}
	return kept
}

// SetMember adds the user, or changes their role if they are already in the list.
func SetMember(members []models.Member, id models.ID, role string) []models.Member {
	for i := range members {
		if members[i].UserID == id {
			members[i].Role = role
			return members
		}
	}
	return append(members, models.Member{UserID: id, Role: role})
}

func MemberIDs(members []models.Member) []models.ID {
	ids := []models.ID{}
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	return ids
}
