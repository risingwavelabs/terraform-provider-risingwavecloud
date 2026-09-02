package cloudsdk

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen"
	apigen_mgmtv2 "github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/cloudsdk/apigen/mgmt/v2"
	"github.com/risingwavelabs/terraform-provider-risingwavecloud/internal/utils/wait"
)

// The statuses a tenant extension reports. The OpenAPI spec types `status` as a plain string
// rather than an enum, so these are transcribed from the platform's own
// `internal/model/extensions.go` and will not follow it if it grows another one. Getting the
// enum into the spec is tracked separately.
const (
	ExtensionStatusEnabling  = "Enabling"
	ExtensionStatusUpdating  = "Updating"
	ExtensionStatusUpgrading = "Upgrading"
	ExtensionStatusDisabling = "Disabling"
	ExtensionStatusRunning   = "Running"
	ExtensionStatusDisabled  = "Disabled"
	ExtensionStatusFailed    = "Failed"
)

// ErrExtensionDisabled reports an extension that is not enabled. Reading one that was never
// enabled is not an error on the platform: it answers `200` with a `Disabled` status rather
// than `404`, and the resources turn that into this so they can drop themselves from state
// the same way they would for a deleted object.
var ErrExtensionDisabled = errors.New("the extension is disabled")

// PollingExtensionOperation bounds the waits. Enabling an extension provisions nodes, so it
// takes minutes rather than the seconds an access-control change takes.
var PollingExtensionOperation = wait.PollingParams{
	Timeout:  15 * time.Minute,
	Interval: 3 * time.Second,
}

// extensionStatus reads the status of one extension. Each extension has its own payload but
// they share a lifecycle, so the waits below work in terms of this alone.
type extensionStatus func(ctx context.Context, nsID uuid.UUID) (string, error)

// pollExtension checks once before polling, since wait.Poll sleeps for a whole interval
// before its first attempt and every mutation would otherwise pay for it twice.
func pollExtension(ctx context.Context, check func() (bool, error)) error {
	if done, err := check(); err != nil || done {
		return err
	}
	return wait.Poll(ctx, check, PollingExtensionOperation)
}

// waitExtensionSettled waits until the extension is no longer mid-flight, which is what the
// platform demands before it will accept another request: it answers one that overlaps
// another with `Illegal status, already exist running workflow`.
//
// `Failed` counts as settled here. It is not a state the extension is working its way out
// of, and refusing to proceed would leave a failed extension impossible to disable through
// terraform -- the one operation that would clear it.
func (c *RegionServiceClient) waitExtensionSettled(ctx context.Context, nsID uuid.UUID, name string, status extensionStatus) error {
	settled := []string{ExtensionStatusRunning, ExtensionStatusDisabled, ExtensionStatusFailed}

	var current string
	if err := pollExtension(ctx, func() (bool, error) {
		s, err := status(ctx, nsID)
		if err != nil {
			return false, err
		}
		current = s
		return slices.Contains(settled, s), nil
	}); err != nil {
		return errors.Wrapf(err, "failed to wait for the %s extension to settle, current status: %s", name, current)
	}
	return nil
}

// waitExtensionApplied waits for a request that has been accepted to take effect.
//
// Unlike the wait above, `Failed` is terminal: the platform has already decided, and polling
// on would only spend the whole budget to report a timeout instead of the failure it could
// have reported immediately. This is the same reason the allowed IAM roles wait gives up on
// `Failed`, and why a failed cluster creation still reports a timeout fifteen minutes late.
func (c *RegionServiceClient) waitExtensionApplied(
	ctx context.Context, nsID uuid.UUID, name string, status extensionStatus, want string,
) error {
	var current string
	if err := pollExtension(ctx, func() (bool, error) {
		s, err := status(ctx, nsID)
		if err != nil {
			return false, err
		}
		current = s
		if s == ExtensionStatusFailed {
			return false, errors.Errorf("the platform failed to apply the %s extension of cluster %s", name, nsID)
		}
		return s == want, nil
	}); err != nil {
		return errors.Wrapf(err, "failed to wait for the %s extension to become %s, current status: %s", name, want, current)
	}
	return nil
}

// extensionPrecondition runs before every mutation: the platform requires the cluster itself
// to be running before it will touch an extension, and requires the extension not to be
// mid-flight.
func (c *RegionServiceClient) extensionPrecondition(ctx context.Context, nsID uuid.UUID, name string, status extensionStatus) error {
	if err := c.waitClusterIdle(ctx, nsID); err != nil {
		return errors.Wrapf(err, "the cluster is not ready to change its %s extension", name)
	}
	return c.waitExtensionSettled(ctx, nsID, name, status)
}

// expectExtensionResponse maps the answers the extension endpoints share. A standalone
// cluster is rejected with `412`, which is worth naming: nothing in the configuration hints
// that the tier decides this.
func expectExtensionResponse(res apigen.SpecResponse, body []byte, nsID uuid.UUID, name string, want int) error {
	switch res.StatusCode() {
	case http.StatusNotFound:
		// The extension endpoints answer 404 for an extension that is not enabled as well as for
		// a cluster that does not exist, and the body is the only thing that tells them apart.
		// Reporting the first as a missing cluster sent people looking for a cluster that was
		// sitting right there.
		if strings.Contains(strings.ToLower(string(body)), "extension") {
			return errors.Wrapf(ErrExtensionDisabled, "the %s extension of cluster %s is not enabled", name, nsID)
		}
		return errors.Wrapf(ErrClusterNotFound, "cluster %s not found", nsID)
	case http.StatusPreconditionFailed:
		return errors.Errorf(
			"the platform refused the %s extension for cluster %s: %s. "+
				"Extensions need a cluster with a separate compute component, which a standalone cluster does not have",
			name, nsID, strings.TrimSpace(string(body)),
		)
	}
	return apigen.ExpectStatusCodeWithMessage(res, want, string(body))
}

//
// Serverless compaction.
//

const extensionServerlessCompaction = "serverless compaction"

// GetServerlessCompaction reports the serverless compaction extension of a cluster. It
// returns ErrExtensionDisabled when the extension is not enabled.
func (c *RegionServiceClient) GetServerlessCompaction(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.GetTenantExtensionCompactionResponseBody, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdExtensionsCompactionWithResponse(ctx, nsID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call API to get the %s extension", extensionServerlessCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessCompaction, http.StatusOK); err != nil {
		return nil, err
	}
	if res.JSON200.Status == ExtensionStatusDisabled {
		return nil, ErrExtensionDisabled
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) serverlessCompactionStatus(ctx context.Context, nsID uuid.UUID) (string, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdExtensionsCompactionWithResponse(ctx, nsID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to call API to get the %s extension", extensionServerlessCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessCompaction, http.StatusOK); err != nil {
		return "", err
	}
	return res.JSON200.Status, nil
}

// EnableServerlessCompactionAwait enables the extension and waits for it to run.
func (c *RegionServiceClient) EnableServerlessCompactionAwait(
	ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessCompactionRequest,
) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionServerlessCompaction, c.serverlessCompactionStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.PostTenantsNsIdExtensionsCompactionWithResponse(ctx, nsID, req)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to enable the %s extension", extensionServerlessCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessCompaction, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionServerlessCompaction, c.serverlessCompactionStatus, ExtensionStatusRunning)
}

// UpdateServerlessCompactionAwait changes the extension and waits for it to run again.
func (c *RegionServiceClient) UpdateServerlessCompactionAwait(
	ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessCompactionRequest,
) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionServerlessCompaction, c.serverlessCompactionStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.PutTenantsNsIdExtensionsCompactionWithResponse(ctx, nsID, req)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to update the %s extension", extensionServerlessCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessCompaction, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionServerlessCompaction, c.serverlessCompactionStatus, ExtensionStatusRunning)
}

// DisableServerlessCompactionAwait disables the extension and waits for it to be gone.
func (c *RegionServiceClient) DisableServerlessCompactionAwait(ctx context.Context, nsID uuid.UUID) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionServerlessCompaction, c.serverlessCompactionStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.DeleteTenantsNsIdExtensionsCompactionWithResponse(ctx, nsID)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to disable the %s extension", extensionServerlessCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessCompaction, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionServerlessCompaction, c.serverlessCompactionStatus, ExtensionStatusDisabled)
}

//
// Serverless backfill.
//

const extensionServerlessBackfill = "serverless backfill"

// GetServerlessBackfill reports the serverless backfill extension of a cluster. It returns
// ErrExtensionDisabled when the extension is not enabled.
func (c *RegionServiceClient) GetServerlessBackfill(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.GetTenantExtensionServerlessBackfillResponseBody, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdExtensionsServerlessBackfillingWithResponse(ctx, nsID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call API to get the %s extension", extensionServerlessBackfill)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessBackfill, http.StatusOK); err != nil {
		return nil, err
	}
	if res.JSON200.Status == ExtensionStatusDisabled {
		return nil, ErrExtensionDisabled
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) serverlessBackfillStatus(ctx context.Context, nsID uuid.UUID) (string, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdExtensionsServerlessBackfillingWithResponse(ctx, nsID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to call API to get the %s extension", extensionServerlessBackfill)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessBackfill, http.StatusOK); err != nil {
		return "", err
	}
	return res.JSON200.Status, nil
}

// EnableServerlessBackfillAwait enables the extension and waits for it to run.
func (c *RegionServiceClient) EnableServerlessBackfillAwait(
	ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessBackfillRequest,
) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionServerlessBackfill, c.serverlessBackfillStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.PostTenantsNsIdExtensionsServerlessBackfillingWithResponse(ctx, nsID, req)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to enable the %s extension", extensionServerlessBackfill)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessBackfill, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionServerlessBackfill, c.serverlessBackfillStatus, ExtensionStatusRunning)
}

// UpdateServerlessBackfillAwait changes the extension and waits for it to run again.
func (c *RegionServiceClient) UpdateServerlessBackfillAwait(
	ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.TenantExtensionServerlessBackfillRequest,
) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionServerlessBackfill, c.serverlessBackfillStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.PutTenantsNsIdExtensionsServerlessBackfillingWithResponse(ctx, nsID, req)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to update the %s extension", extensionServerlessBackfill)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessBackfill, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionServerlessBackfill, c.serverlessBackfillStatus, ExtensionStatusRunning)
}

// DisableServerlessBackfillAwait disables the extension and waits for it to be gone.
func (c *RegionServiceClient) DisableServerlessBackfillAwait(ctx context.Context, nsID uuid.UUID) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionServerlessBackfill, c.serverlessBackfillStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.DeleteTenantsNsIdExtensionsServerlessBackfillingWithResponse(ctx, nsID)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to disable the %s extension", extensionServerlessBackfill)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionServerlessBackfill, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionServerlessBackfill, c.serverlessBackfillStatus, ExtensionStatusDisabled)
}

//
// Iceberg compaction.
//

const extensionIcebergCompaction = "iceberg compaction"

// GetIcebergCompaction reports the iceberg compaction extension of a cluster. It returns
// ErrExtensionDisabled when the extension is not enabled.
func (c *RegionServiceClient) GetIcebergCompaction(ctx context.Context, nsID uuid.UUID) (*apigen_mgmtv2.IcebergCompaction, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdExtensionsIcebergCompactionWithResponse(ctx, nsID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to call API to get the %s extension", extensionIcebergCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionIcebergCompaction, http.StatusOK); err != nil {
		return nil, err
	}
	if res.JSON200.Status == ExtensionStatusDisabled {
		return nil, ErrExtensionDisabled
	}
	return res.JSON200, nil
}

func (c *RegionServiceClient) icebergCompactionStatus(ctx context.Context, nsID uuid.UUID) (string, error) {
	res, err := c.mgmtV2Client.GetTenantsNsIdExtensionsIcebergCompactionWithResponse(ctx, nsID)
	if err != nil {
		return "", errors.Wrapf(err, "failed to call API to get the %s extension", extensionIcebergCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionIcebergCompaction, http.StatusOK); err != nil {
		return "", err
	}
	return res.JSON200.Status, nil
}

// EnableIcebergCompactionAwait enables the extension and waits for it to run.
func (c *RegionServiceClient) EnableIcebergCompactionAwait(
	ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PostTenantsNsIdExtensionsIcebergCompactionJSONRequestBody,
) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionIcebergCompaction, c.icebergCompactionStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.PostTenantsNsIdExtensionsIcebergCompactionWithResponse(ctx, nsID, req)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to enable the %s extension", extensionIcebergCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionIcebergCompaction, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionIcebergCompaction, c.icebergCompactionStatus, ExtensionStatusRunning)
}

// UpdateIcebergCompactionAwait changes the extension and waits for it to run again.
func (c *RegionServiceClient) UpdateIcebergCompactionAwait(
	ctx context.Context, nsID uuid.UUID, req apigen_mgmtv2.PutTenantsNsIdExtensionsIcebergCompactionJSONRequestBody,
) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionIcebergCompaction, c.icebergCompactionStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.PutTenantsNsIdExtensionsIcebergCompactionWithResponse(ctx, nsID, req)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to update the %s extension", extensionIcebergCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionIcebergCompaction, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionIcebergCompaction, c.icebergCompactionStatus, ExtensionStatusRunning)
}

// DisableIcebergCompactionAwait disables the extension and waits for it to be gone.
func (c *RegionServiceClient) DisableIcebergCompactionAwait(ctx context.Context, nsID uuid.UUID) error {
	if err := c.extensionPrecondition(ctx, nsID, extensionIcebergCompaction, c.icebergCompactionStatus); err != nil {
		return err
	}
	res, err := c.mgmtV2Client.DeleteTenantsNsIdExtensionsIcebergCompactionWithResponse(ctx, nsID)
	if err != nil {
		return errors.Wrapf(err, "failed to call API to disable the %s extension", extensionIcebergCompaction)
	}
	if err := expectExtensionResponse(res, res.Body, nsID, extensionIcebergCompaction, http.StatusAccepted); err != nil {
		return err
	}
	return c.waitExtensionApplied(ctx, nsID, extensionIcebergCompaction, c.icebergCompactionStatus, ExtensionStatusDisabled)
}
