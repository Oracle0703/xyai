package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceAdminMapsDepartmentManagementFields(t *testing.T) {
	source := "department_manager"
	input := &service.User{
		ID:             42,
		Status:         service.StatusDisabled,
		Notes:          "administrator note",
		DepartmentNote: "department manager note",
		DisabledSource: &source,
	}

	output := UserFromServiceAdmin(input)
	require.Equal(t, input.DepartmentNote, output.DepartmentNote)
	require.Equal(t, input.DisabledSource, output.DisabledSource)
	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Equal(t, input.DepartmentNote, fields["department_note"])
	require.Equal(t, source, fields["disabled_source"])

	input.Status, input.DisabledSource = service.StatusActive, nil
	encoded, err = json.Marshal(UserFromServiceAdmin(input))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Contains(t, fields, "disabled_source")
	require.Nil(t, fields["disabled_source"])
}

func TestUserDTOsDoNotExposeDepartmentManagementFields(t *testing.T) {
	source := "department_manager"
	input := &service.User{
		ID:             42,
		Notes:          "administrator note",
		DepartmentNote: "department manager note",
		DisabledSource: &source,
	}
	for name, value := range map[string]any{
		"user":         UserFromService(input),
		"shallow":      UserFromServiceShallow(input),
		"api key":      APIKeyFromService(&service.APIKey{User: input}),
		"subscription": UserSubscriptionFromService(&service.UserSubscription{User: input}),
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			for _, field := range []string{"department_note", "disabled_source", "administrator note", "department manager note"} {
				require.NotContains(t, string(encoded), field)
			}
		})
	}
}
