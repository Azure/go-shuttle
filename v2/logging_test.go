package shuttle

import (
	"bytes"
	"log/slog"
	"testing"

	. "github.com/onsi/gomega"
)

func TestSetSlogHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	SetSlogHandler(slog.NewTextHandler(buf, nil))

	getLogger().Info("testInfo")
	g := NewWithT(t)
	g.Expect(buf.String()).To(ContainSubstring("testInfo"))
}
