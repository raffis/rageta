package xio

// Stream IDs used to multiplex a task's real output together with
// out-of-band stats samples onto a single fd (see cmd/shim), and to demux
// them back apart on the consuming side.
const (
	StreamOutput byte = 1
	StreamStats  byte = 2
	StreamIP     byte = 3
)
