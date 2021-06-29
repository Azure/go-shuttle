package propagation

type TraceType int

const (
	None          TraceType = iota
	OpenTelemetry TraceType = iota
	OpenCensus    TraceType = iota
)

func (t TraceType) String() string {
	switch t {
	case None:
		return "None"
	case OpenTelemetry:
		return "OpenTelemetry"
	case OpenCensus:
		return "OpenCensus"
	default:
		return "Undefined"
	}
}
