package handlers

import "fmt"

var NextHandlerNilError error = fmt.Errorf("next handler cannot be nil")
