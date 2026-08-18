package provider

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
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

// fakeAllowedIamRolesClient records the order of the calls a change makes.
type fakeAllowedIamRolesClient struct {
	cloudsdk.CloudClientInterface

	current []string
	calls   []string
}

func (c *fakeAllowedIamRolesClient) GetAllowedIamRoles(ctx context.Context, nsID uuid.UUID) ([]string, error) {
	return c.current, nil
}

func (c *fakeAllowedIamRolesClient) AddAllowedIamRoleAwait(ctx context.Context, nsID uuid.UUID, roleArn string) error {
	c.calls = append(c.calls, "add "+roleArn)
	return nil
}

func (c *fakeAllowedIamRolesClient) RemoveAllowedIamRoleAwait(ctx context.Context, nsID uuid.UUID, roleArn string) error {
	c.calls = append(c.calls, "remove "+roleArn)
	return nil
}

// Replacing the list must free the outgoing entries before claiming room for the incoming
// ones: holding both at once runs into the platform's maximum, and it would leave the roles on
// their way out allowed alongside their replacements.
func TestApplyAllowedIamRolesRemovesBeforeAdding(t *testing.T) {
	const (
		keep = "arn:aws:iam::123456789012:role/keep"
		gone = "arn:aws:iam::123456789012:role/gone"
		want = "arn:aws:iam::123456789012:role/want"
	)

	client := &fakeAllowedIamRolesClient{current: []string{keep, gone}}
	r := &ClusterAllowedIamRolesResource{client: client}

	err := r.applyAllowedIamRoles(context.Background(), uuid.Must(uuid.NewRandom()), client.current, []string{keep, want})
	require.NoError(t, err)

	assert.Equal(t, []string{"remove " + gone, "add " + want}, client.calls)
}
