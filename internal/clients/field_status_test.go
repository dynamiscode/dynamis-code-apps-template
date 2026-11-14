package clients

import (
	"errors"
	"testing"
	"time"
)

func TestFieldStatusFromArchiveTime(t *testing.T) {
	archived := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := (Field{}).Status(); got != FieldStatusActive {
		t.Fatalf("status without archive time = %q, want %q", got, FieldStatusActive)
	}
	if got := (Field{ArchivedAt: &archived}).Status(); got != FieldStatusArchived {
		t.Fatalf("status with archive time = %q, want %q", got, FieldStatusArchived)
	}
}

// TestValidateFieldTransition is the field lifecycle table. A deleted field has
// no row, so deletion is the only way out of archived, and nothing leaves deleted.
func TestValidateFieldTransition(t *testing.T) {
	tests := []struct {
		name            string
		current, target FieldStatus
		wantErr         error
	}{
		{"archive an active field", FieldStatusActive, FieldStatusArchived, nil},
		{"restore an archived field", FieldStatusArchived, FieldStatusActive, nil},
		{"delete an archived field", FieldStatusArchived, FieldStatusDeleted, nil},
		{"delete an active field needs archive first", FieldStatusActive, FieldStatusDeleted, ErrFieldNotArchived},
		{"active to active is not a transition", FieldStatusActive, FieldStatusActive, errInvalidFieldTransition},
		{"archived to archived is not a transition", FieldStatusArchived, FieldStatusArchived, errInvalidFieldTransition},
		{"deleted fields do not come back", FieldStatusDeleted, FieldStatusActive, errInvalidFieldTransition},
		{"deleted fields do not archive", FieldStatusDeleted, FieldStatusArchived, errInvalidFieldTransition},
		{"deleted to deleted is not a transition", FieldStatusDeleted, FieldStatusDeleted, errInvalidFieldTransition},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFieldTransition(tc.current, tc.target)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateFieldTransition(%s, %s) = %v, want nil", tc.current, tc.target, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateFieldTransition(%s, %s) = %v, want %v", tc.current, tc.target, err, tc.wantErr)
			}
		})
	}
}
