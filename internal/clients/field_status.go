package clients

import "errors"

// FieldStatus is a custom field's lifecycle state. A deleted field has no row,
// so deleted is a transition target and never a value that Status returns.
type FieldStatus string

const (
	FieldStatusActive   FieldStatus = "active"
	FieldStatusArchived FieldStatus = "archived"
	FieldStatusDeleted  FieldStatus = "deleted"
)

var errInvalidFieldTransition = errors.New("client field transition is not allowed")

// Status returns the field's current status. Fields have no time-based states,
// so the status depends only on the archive time.
func (f Field) Status() FieldStatus {
	if f.ArchivedAt != nil {
		return FieldStatusArchived
	}
	return FieldStatusActive
}

// status is the field's status as the value and delete paths read it.
func (r fieldRef) status() FieldStatus {
	if r.archived {
		return FieldStatusArchived
	}
	return FieldStatusActive
}

// ValidateFieldTransition enforces the field lifecycle: archive and restore
// swap active and archived, and delete leaves archived only. Deleting an active
// field returns ErrFieldNotArchived, the error the delete path has always named.
func ValidateFieldTransition(current, target FieldStatus) error {
	switch {
	case current == FieldStatusActive && target == FieldStatusArchived,
		current == FieldStatusArchived && target == FieldStatusActive,
		current == FieldStatusArchived && target == FieldStatusDeleted:
		return nil
	case current == FieldStatusActive && target == FieldStatusDeleted:
		return ErrFieldNotArchived
	}
	return errInvalidFieldTransition
}
