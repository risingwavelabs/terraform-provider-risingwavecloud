package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

type DataExtractHelperInterface interface {
	Get(ctx context.Context, getter DataGetter, target interface{}) diag.Diagnostics
	Set(ctx context.Context, setter DataSetter, val interface{}) diag.Diagnostics
}

type DataGetter interface {
	Get(ctx context.Context, target interface{}) diag.Diagnostics
}

type DataSetter interface {
	Set(ctx context.Context, val interface{}) diag.Diagnostics
}

type DataExtractHelper struct {
}

func (d *DataExtractHelper) Get(ctx context.Context, getter DataGetter, target interface{}) diag.Diagnostics {
	return getter.Get(ctx, target)
}

func (d *DataExtractHelper) Set(ctx context.Context, setter DataSetter, val interface{}) diag.Diagnostics {
	return setter.Set(ctx, val)
}
