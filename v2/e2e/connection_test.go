package e2e

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	endpointKey            = "Endpoint"
	sharedAccessKeyNameKey = "SharedAccessKeyName"
	sharedAccessKeyKey     = "SharedAccessKey"
	entityPathKey          = "EntityPath"
)

type (
	// ParsedConn is the structure of a parsed Service Bus or Event Hub connection string.
	ParsedConn struct {
		Host      string
		Suffix    string
		Namespace string
		HubName   string
		KeyName   string
		Key       string
	}
)

// newParsedConnection is a constructor for a parsedConn and verifies each of the inputs is non-null.
func newParsedConnection(namespace, suffix, hubName, keyName, key string) *ParsedConn {
	return &ParsedConn{
		Host:      "amqps://" + namespace + "." + suffix,
		Suffix:    suffix,
		Namespace: namespace,
		KeyName:   keyName,
		Key:       key,
		HubName:   hubName,
	}
}

// parsedConnectionFromStr takes a string connection string from the Azure portal and returns the parsed representation.
// The method will return an error if the Endpoint, SharedAccessKeyName or SharedAccessKey is empty.
func parsedConnectionFromStr(connStr string) (*ParsedConn, error) {
	var namespace, suffix, hubName, keyName, secret string
	splits := strings.Split(connStr, ";")
	for _, split := range splits {
		keyAndValue := strings.Split(split, "=")
		if len(keyAndValue) < 2 {
			return nil, errors.New("failed parsing connection string due to unmatched key value separated by '='")
		}

		// if a key value pair has `=` in the value, recombine them
		key := keyAndValue[0]
		value := strings.Join(keyAndValue[1:], "=")
		switch {
		case strings.EqualFold(endpointKey, key):
			u, err := url.Parse(value)
			if err != nil {
				return nil, errors.New("failed parsing connection string due to an incorrectly formatted Endpoint value")
			}
			hostSplits := strings.Split(u.Host, ".")
			if len(hostSplits) < 2 {
				return nil, errors.New("failed parsing connection string due to Endpoint value not containing a URL with a namespace and a suffix")
			}
			namespace = hostSplits[0]
			suffix = strings.Join(hostSplits[1:], ".")
		case strings.EqualFold(sharedAccessKeyNameKey, key):
			keyName = value
		case strings.EqualFold(sharedAccessKeyKey, key):
			secret = value
		case strings.EqualFold(entityPathKey, key):
			hubName = value
		}
	}

	parsed := newParsedConnection(namespace, suffix, hubName, keyName, secret)
	if namespace == "" {
		return parsed, fmt.Errorf("key %q must not be empty", endpointKey)
	}

	if keyName == "" {
		return parsed, fmt.Errorf("key %q must not be empty", sharedAccessKeyNameKey)
	}

	if secret == "" {
		return parsed, fmt.Errorf("key %q must not be empty", sharedAccessKeyKey)
	}

	return parsed, nil
}
