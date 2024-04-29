package shuttle

import (
	"bytes"
	"log/slog"
	"testing"

	. "github.com/onsi/gomega"
)

func TestSetSlogHandler(t *testing.T) {
	g := NewWithT(t)
	g.Expect(func() { SetLogHandler(nil) }).ToNot(Panic())

	buf := &bytes.Buffer{}
	SetLogHandler(slog.NewTextHandler(buf, nil))
	getLogger().Info("testInfo")
	g.Expect(buf.String()).To(ContainSubstring("testInfo"))
}
