package web

// ProtocolVersion identifies the private browser protocol understood by this
// binary's aggregate source boundary.
const ProtocolVersion uint32 = 1

const (
	// CapabilityBoardCatalog identifies source bootstrap and board catalog reads.
	CapabilityBoardCatalog = "board.catalog"

	// CapabilityBoardRead identifies canonical board detail reads.
	CapabilityBoardRead = "board.read"

	// CapabilityIssueRead identifies issue collection and detail reads.
	CapabilityIssueRead = "issue.read"

	// CapabilityLogRead identifies immutable issue log reads.
	CapabilityLogRead = "log.read"

	// CapabilityApprovalRead identifies actionable checkpoint reads.
	CapabilityApprovalRead = "approval.read"

	// CapabilityRoutineRead identifies routine collection reads.
	CapabilityRoutineRead = "routine.read"

	// CapabilityChangeRead identifies source change invalidation streams.
	CapabilityChangeRead = "change.read"
)

// ReadCapabilities returns the capabilities required by aggregate sources.
func ReadCapabilities() []string {
	return []string{
		CapabilityBoardCatalog,
		CapabilityBoardRead,
		CapabilityIssueRead,
		CapabilityLogRead,
		CapabilityApprovalRead,
		CapabilityRoutineRead,
		CapabilityChangeRead,
	}
}
