package provider

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmtv1 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v1"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	cloudsdk_mock "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/mock"
	"github.com/stretchr/testify/assert"
)

func createSimpleTestCluster(t *testing.T, name, region, imageTag string, tier apigen_mgmtv2.TierId, status apigen_mgmtv2.TenantStatus) *apigen_mgmtv2.Tenant {
	t.Helper()

	return &apigen_mgmtv2.Tenant{
		Id:         1,
		ImageTag:   imageTag,
		NsId:       uuid.Must(uuid.NewRandom()),
		Region:     region,
		TenantName: name,
		Tier:       tier,
		Status:     status,
		Resources: apigen_mgmtv2.TenantResource{
			Components: apigen_mgmtv2.TenantResourceComponents{
				Compactor: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Compute: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Frontend: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
				Meta: &apigen_mgmtv2.ComponentResource{
					ComponentTypeId: "p-1c4g",
					Cpu:             "1",
					Memory:          "4 GB",
					Replica:         1,
				},
			},
		},
	}
}

func TestMetaStoreEqual(t *testing.T) {
	tests := []struct {
		name string
		a    *apigen_mgmtv2.TenantResourceMetaStore
		b    *apigen_mgmtv2.TenantResourceMetaStore
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a nil b non-nil",
			a:    nil,
			b:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.AwsRds},
			want: true,
		},
		{
			name: "a non-nil b nil",
			a:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.AwsRds},
			b:    nil,
			want: true,
		},
		{
			name: "same type",
			a:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.AwsRds},
			b:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.AwsRds},
			want: true,
		},
		{
			name: "same type different rwu",
			a:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.AwsRds, Rwu: "2"},
			b:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.AwsRds, Rwu: ""},
			want: true,
		},
		{
			name: "different type",
			a:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.AwsRds},
			b:    &apigen_mgmtv2.TenantResourceMetaStore{Type: apigen_mgmtv2.Etcd},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metaStoreEqual(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTierValidation(t *testing.T) {
	tests := []struct {
		name    string
		tier    string
		isBYOC  bool
		wantErr bool
		errMsg  string
	}{
		{
			name:   "SaaS Standard",
			tier:   string(apigen_mgmtv2.TierIdStandard),
			isBYOC: false,
		},
		{
			name:   "SaaS Invited",
			tier:   string(apigen_mgmtv2.TierIdInvited),
			isBYOC: false,
		},
		{
			name:    "SaaS with BYOC tier",
			tier:    string(apigen_mgmtv2.TierIdBYOC),
			isBYOC:  false,
			wantErr: true,
			errMsg:  "SaaS clusters must use either Standard or Invited tier",
		},
		{
			name:   "BYOC with BYOC tier",
			tier:   string(apigen_mgmtv2.TierIdBYOC),
			isBYOC: true,
		},
		{
			name:    "BYOC with Standard tier",
			tier:    string(apigen_mgmtv2.TierIdStandard),
			isBYOC:  true,
			wantErr: true,
			errMsg:  "BYOC clusters must use the BYOC tier",
		},
		{
			name:    "BYOC with Invited tier",
			tier:    string(apigen_mgmtv2.TierIdInvited),
			isBYOC:  true,
			wantErr: true,
			errMsg:  "BYOC clusters must use the BYOC tier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier := apigen_mgmtv2.TierId(tt.tier)
			var hasErr bool
			var errMsg string

			if tt.isBYOC && tier != apigen_mgmtv2.TierIdBYOC {
				hasErr = true
				errMsg = "BYOC clusters must use the BYOC tier"
			}
			if !tt.isBYOC && tier != apigen_mgmtv2.TierIdStandard && tier != apigen_mgmtv2.TierIdInvited {
				hasErr = true
				errMsg = "SaaS clusters must use either Standard or Invited tier"
			}

			assert.Equal(t, tt.wantErr, hasErr)
			if tt.wantErr {
				assert.Contains(t, errMsg, tt.errMsg)
			}
		})
	}
}

func TestClusterCreate_previous_creation_failed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var (
		ctx      = context.Background()
		name     = "test-cluster"
		region   = "us-west-2"
		imageTag = "v1.10.0"
		tier     = apigen_mgmtv2.TierIdStandard
		tierV1   = apigen_mgmtv1.TierId(tier)
		tenant   = createSimpleTestCluster(t, name, region, imageTag, tier, apigen_mgmtv2.Failed)
	)

	client := cloudsdk_mock.NewMockCloudClientInterface(ctrl)

	dataHelper := NewMockDataExtractHelperInterface(ctrl)

	dataHelper.EXPECT().
		Get(ctx, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, getter DataGetter, target interface{}) diag.Diagnostics {
			p, ok := target.(*ClusterModel)
			assert.True(t, ok)
			clusterToDataModel(tenant, nil, p)
			return nil
		})
	dataHelper.EXPECT().
		Set(ctx, gomock.Any(), gomock.Any()).
		Return(nil)

	client.
		EXPECT().
		GetClusterByRegionAndName(ctx, region, name).
		Return(tenant, nil)

	client.
		EXPECT().
		DeleteClusterByNsIDAwait(ctx, tenant.NsId).
		Return(nil)

	client.
		EXPECT().
		GetAvailableComponentTypes(ctx, region, tierV1, gomock.Any()).
		Return([]apigen_mgmtv1.AvailableComponentType{
			{
				Id:      "p-1c4g",
				Maximum: 3,
				Cpu:     "1",
				Memory:  "4 GB",
			},
		}, nil).
		Times(4)

	client.
		EXPECT().
		CreateClusterAwait(ctx, region, gomock.Any()).
		DoAndReturn(func(ctx context.Context, region string, req apigen_mgmtv2.TenantRequestRequestBody) (*apigen_mgmtv2.Tenant, error) {
			rtn := *tenant
			rtn.Status = apigen_mgmtv2.Running
			return &rtn, nil
		})

	client.
		EXPECT().
		GetBYOCCluster(ctx, region, "").
		Return(nil, cloudsdk.ErrBYOCClusterNotFound)

	p := &ClusterResource{
		client:     client,
		dataHelper: dataHelper,
	}

	p.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{},
	}, &resource.CreateResponse{
		State: tfsdk.State{},
	})
}
