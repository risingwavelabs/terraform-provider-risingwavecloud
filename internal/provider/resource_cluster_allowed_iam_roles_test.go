package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The platform rejects a malformed ARN with `validate arn failed: invalid target format,
// expected format: arn:aws:iam::{account}:role/{role_name}`. Checking the same shape here is
// what turns that into a plan-time error.
func TestRoleArnValidator(t *testing.T) {
	tests := []struct {
		arn   string
		valid bool
	}{
		{arn: "arn:aws:iam::123456789012:role/my-role", valid: true},
		{arn: "arn:aws:iam::123456789012:role/path/to/my-role", valid: true},
		{arn: "arn:aws:iam::123456789012:role/My.Role_1+2=3@4", valid: true},
		{arn: "not-an-arn", valid: false},
		{arn: "", valid: false},
		{arn: "arn:aws:iam::123456789012:user/my-user", valid: false},
		{arn: "arn:aws:iam::12345:role/my-role", valid: false},           // account is 12 digits
		{arn: "arn:aws-cn:iam::123456789012:role/my-role", valid: false}, // the platform states the aws partition
		{arn: "arn:aws:iam::123456789012:role/", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.arn, func(t *testing.T) {
			resp := &validator.StringResponse{}
			roleArnValidator{}.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("role_arns"),
				ConfigValue: types.StringValue(tt.arn),
			}, resp)

			assert.Equal(t, tt.valid, !resp.Diagnostics.HasError())
		})
	}
}

// Every element of the set is checked, not just the first.
func TestRoleArnSetValidator(t *testing.T) {
	ctx := context.Background()

	value, diags := types.SetValueFrom(ctx, types.StringType, []string{
		"arn:aws:iam::123456789012:role/good",
		"nonsense",
	})
	require.False(t, diags.HasError())

	resp := &validator.SetResponse{}
	setValidatorEach{roleArnValidator{}}.ValidateSet(ctx, validator.SetRequest{
		Path:        path.Root("role_arns"),
		ConfigValue: value,
	}, resp)

	assert.True(t, resp.Diagnostics.HasError(), "a bad element anywhere in the set must be reported")
}
