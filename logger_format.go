package fiberserver

import (
	"bytes"
	"context"
	"fmt"

	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/tenant"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/xchannelrequestid"
	"github.com/netcracker/qubership-core-lib-go/v3/context-propagation/baseproviders/xrequestid"
	"github.com/netcracker/qubership-core-lib-go/v3/logging"
)

func init() {
	logging.DefaultFormat.SetMessageFormat(platformMessageFmt)
}

func platformMessageFmt(r *logging.Record, b *bytes.Buffer, color int, lvl string) (int, error) {
	timeFormat := "2006-01-02T15:04:05.000"
	return fmt.Fprintf(b, "[%s] [%s] [request_id=%s] [tenant_id=%s] [thread=%s] [class=%s] [x_channel_request_id=%s] %s",
		r.Time.Format(timeFormat),
		lvl,
		getRequestId(r.Ctx),
		getTenantId(r.Ctx),
		getContextIdentifier(r.Ctx),
		logging.ConstructCallerValueByRecord(r),
		getXChannelRequestId(r.Ctx),
		logging.JoinStringsWithSpace(logging.AssembleDefaultCustomLogFields(r.Ctx), r.Message),
	)
}

func getContextIdentifier(_ context.Context) string {
	return "-"
}

func getRequestId(ctx context.Context) string {
	if ctx != nil {
		abstractRequestId := ctx.Value(xrequestid.X_REQUEST_ID_COTEXT_NAME)
		if abstractRequestId != nil {
			requestId := abstractRequestId.(xrequestid.XRequestId)
			return requestId.GetRequestId()
		}
	}
	return "-"
}

func getTenantId(ctx context.Context) string {
	if ctx != nil {
		abstractTenantId := ctx.Value(tenant.TenantContextName)
		if abstractTenantId != nil {
			tenantId := abstractTenantId.(tenant.TenantContextObject)
			return tenantId.GetTenant()
		}
	}
	return "-"
}

func getXChannelRequestId(ctx context.Context) string {
	if ctx != nil {
		abstractChannelRequestId := ctx.Value(xchannelrequestid.X_CHANNEL_REQUEST_ID_CONTEXT_NAME)
		if abstractChannelRequestId != nil {
			channelRequestId := abstractChannelRequestId.(xchannelrequestid.XChannelRequestId)
			return channelRequestId.GetChannelRequestId()
		}
	}
	return "-"
}
