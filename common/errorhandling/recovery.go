package errorhandling

import (
	"errors"
	"io"
	"net"
	"strings"

	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-amqp"
)

// NOTE: Although the error message says that the operation can be retried, amqp:internal-error has been found to be persistent until we rebuild the connection (i.e: restart the process)
// sample error :
// *Error{
//    Condition: amqp:internal-error,
//    Description: The service was unable to process the request; please retry the operation.
//    For more information on exception types and proper exception handling, please refer to http://go.microsoft.com/fwlink/?LinkId=761101
//    Reference:<REDACTED>,
//    TrackingId:<REDACTED>,
//    SystemTracker:<REDACTED> Topic:<REDACTED>, Timestamp:2021-06-19T23:17:15, Info: map[]
// }
func isAmqpInternalError(err error) bool {
	var amqpErr *amqp.Error
	return errors.As(err, &amqpErr) &&
		amqpErr.Condition == amqp.ErrorInternalError &&
		strings.HasPrefix("the service was unable to process the request", strings.ToLower(amqpErr.Description))
}

func isPermanentNetError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && (!netErr.Temporary() || netErr.Timeout())
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF)
}

func isConnClosedError(err error) bool {
	errConnClosed := servicebus.ErrConnectionClosed("")
	return errors.As(err, &errConnClosed)
}

func isLinkDetachedError(err error) bool {
	return errors.Is(err, amqp.ErrLinkDetached)
}

func IsConnectionDead(err error) bool {
	return isPermanentNetError(err) ||
		isLinkDetachedError(err) ||
		isAmqpInternalError(err) ||
		isEOF(err) ||
		isConnClosedError(err)
}
