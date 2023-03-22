package shuttle

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	. "github.com/onsi/gomega"
)

func TestSettlementHandler(t *testing.T) {
	tests := []struct {
		name       string
		settlement Settlement
		assert     func(g Gomega, settler *fakeSettler)
		want       HandlerFunc
	}{
		{
			name:       "Complete",
			settlement: &Complete{},
			assert: func(g Gomega, settler *fakeSettler) {
				g.Expect(settler.completed).To(BeTrue())
			},
		},
		{
			name:       "Abandon",
			settlement: &Abandon{},
			assert: func(g Gomega, settler *fakeSettler) {
				g.Expect(settler.abandoned).To(BeTrue())
			},
		},
		{
			name:       "Deadletter",
			settlement: &DeadLetter{},
			assert: func(g Gomega, settler *fakeSettler) {
				g.Expect(settler.deadlettered).To(BeTrue())
			},
		},
		{
			name:       "Defer",
			settlement: &Defer{},
			assert: func(g Gomega, settler *fakeSettler) {
				g.Expect(settler.defered).To(BeTrue())
			},
		},
		{
			name:       "NoOp",
			settlement: &NoOp{},
			assert: func(g Gomega, settler *fakeSettler) {
				g.Expect(settler.completed).To(BeFalse())
				g.Expect(settler.abandoned).To(BeFalse())
				g.Expect(settler.deadlettered).To(BeFalse())
				g.Expect(settler.defered).To(BeFalse())
			},
		},
		{
			name:       "nil settlement panics",
			settlement: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.TODO()
			settler := &fakeSettler{}
			settlementHandler := Settler(func(ctx context.Context, message *azservicebus.ReceivedMessage) Settlement {
				return tt.settlement
			})
			settlmentHandler := NewSettlementHandler(nil, settlementHandler)
			if tt.settlement != nil {
				settlmentHandler.Handle(ctx, settler, &azservicebus.ReceivedMessage{})
				tt.assert(g, settler)
			} else {
				g.Expect(func() { settlmentHandler.Handle(ctx, settler, &azservicebus.ReceivedMessage{}) }).To(Panic())
			}
		})
	}
}
