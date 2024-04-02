package provider

import (
	context "context"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/uuid"
	diag "github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk"
	apigen_mgmt "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt"
	cloudsdk_mock "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/mock"
	"github.com/stretchr/testify/assert"
)

func TestPrivateLinkCreate_previous_creation_failed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	var (
		ctx            = context.Background()
		clusterID      = uuid.Must(uuid.NewRandom())
		connectionName = "my_connection"
		plTarget       = "target"
		plID           = uuid.Must(uuid.NewRandom())
		plInfo         = &cloudsdk.PrivateLinkInfo{
			ClusterNsID: clusterID,
			PrivateLink: &apigen_mgmt.PrivateLink{
				Id:             plID,
				ConnectionName: connectionName,
				Target:         &plTarget,
				Status:         apigen_mgmt.ERROR,
			},
		}
	)

	client := cloudsdk_mock.NewMockCloudClientInterface(ctrl)

	dataHelper := NewMockDataExtractHelperInterface(ctrl)

	dataHelper.EXPECT().
		Get(ctx, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, getter DataGetter, target interface{}) diag.Diagnostics {
			p, ok := target.(*PrivateLinkModel)
			assert.True(t, ok)
			privateLinkToDataModel(plInfo, p)
			return nil
		})

	client.
		EXPECT().
		GetPrivateLinkByName(ctx, connectionName).
		Return(plInfo, nil)

	client.EXPECT().
		DeletePrivateLinkAwait(ctx, clusterID, plID).
		Return(nil)

	client.EXPECT().
		CreatePrivateLinkAwait(ctx, clusterID, apigen_mgmt.PostPrivateLinkRequestBody{
			ConnectionName: connectionName,
			Target:         plTarget,
		}).
		DoAndReturn(func(ctx context.Context, nsID uuid.UUID, req apigen_mgmt.PostPrivateLinkRequestBody) (*cloudsdk.PrivateLinkInfo, error) {
			rtn := *plInfo
			rtn.PrivateLink.Status = apigen_mgmt.CREATED
			return &rtn, nil
		})

	dataHelper.EXPECT().
		Set(ctx, gomock.Any(), gomock.Any()).
		Return(nil)

	p := &PrivateLinkResource{
		client:     client,
		dataHelper: dataHelper,
	}

	p.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{},
	}, &resource.CreateResponse{
		State: tfsdk.State{},
	})
}
