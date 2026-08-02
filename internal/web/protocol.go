package web

import privatev1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"

// BrowserProtocol reports the private browser protocol implemented by this
// binary. The protocol schema owns its version and capability vocabulary.
func BrowserProtocol() *privatev1.WebProtocol {
	return &privatev1.WebProtocol{
		Version: privatev1.WebProtocolVersion_WEB_PROTOCOL_VERSION_V1,
		Capabilities: []privatev1.WebCapability{
			privatev1.WebCapability_WEB_CAPABILITY_BOARD_CATALOG,
			privatev1.WebCapability_WEB_CAPABILITY_BOARD_READ,
			privatev1.WebCapability_WEB_CAPABILITY_ISSUE_READ,
			privatev1.WebCapability_WEB_CAPABILITY_LOG_READ,
			privatev1.WebCapability_WEB_CAPABILITY_STATE_READ,
			privatev1.WebCapability_WEB_CAPABILITY_ATTACHMENT_READ,
			privatev1.WebCapability_WEB_CAPABILITY_APPROVAL_READ,
			privatev1.WebCapability_WEB_CAPABILITY_ROUTINE_READ,
			privatev1.WebCapability_WEB_CAPABILITY_CHANGE_READ,
		},
	}
}
