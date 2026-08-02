package web

// ProtocolVersion identifies the private browser protocol understood by this
// binary's aggregate source boundary.
const ProtocolVersion uint32 = 1

const (
	// CapabilityBoardCatalog identifies source bootstrap and board catalog reads.
	CapabilityBoardCatalog = "board.catalog"

	// CapabilityBoardRead identifies canonical board detail reads.
	CapabilityBoardRead = "board.read"
)

// ReadCapabilities returns the capabilities required by aggregate sources.
func ReadCapabilities() []string {
	return []string{CapabilityBoardCatalog, CapabilityBoardRead}
}
