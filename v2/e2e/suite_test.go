package e2e

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/Azure/azure-amqp-common-go/v3/conn"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	azadmin "github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"
	"github.com/joho/godotenv"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/multierr"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz123456789")

type SBSuite struct {
	suite.Suite
	Prefix         string
	TenantID       string
	SubscriptionID string
	ClientID       string
	ClientSecret   string
	ConnStr        string
	Location       string
	Namespace      string
	ResourceGroup  string
	TagID          string
	closer         io.Closer
	sbAdminClient  *azadmin.Client
	sbClient       *azservicebus.Client
}

func (s *SBSuite) GetSender(queueOrTopic string) (*azservicebus.Sender, error) {
	// prefix the queue/topic
	return s.sbClient.NewSender(queueOrTopic, nil)
}

func init() {
	rand.Seed(time.Now().Unix())
}

// randomString generates a random string with prefix
func randomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}

func TestSuite(t *testing.T) {
    t.Helper()
    if os.Getenv("INTEGRATION") == "" {
        t.Skip("skipping integration tests, set environment variable INTEGRATION")
    }
	suite.Run(t, &SBSuite{Prefix: "v5"})
}

func (s *SBSuite) InitFromEnv() error {
	setFromEnv := func(key string, target *string) error {
		v := os.Getenv(key)
		if v == "" {
			return fmt.Errorf("missing environment variable - %q required for integration tests", key)
		}
		*target = v
		return nil
	}
	return multierr.Combine(
		setFromEnv("AZURE_TENANT_ID", &s.TenantID),
		setFromEnv("AZURE_SUBSCRIPTION_ID", &s.SubscriptionID),
		setFromEnv("AZURE_CLIENT_ID", &s.ClientID),
		setFromEnv("AZURE_CLIENT_SECRET", &s.ClientSecret),
		setFromEnv("SERVICEBUS_CONNECTION_STRING", &s.ConnStr),
		setFromEnv("TEST_RESOURCE_GROUP", &s.ResourceGroup),
		setFromEnv("TEST_LOCATION", &s.Location))
}

func (s *SBSuite) SetupSuite() {
	if err := godotenv.Load("../../.env"); err != nil {
		s.T().Log(err)
	}
	s.T().Setenv("GOSHUTTLE_LOG", "ALL")
	if os.Getenv("TRACING") == "1" {
		_, err := initTracing()
		if err != nil {
			s.FailNow("failed to initialize tracing: %s", err)
		}
	}
	err := s.InitFromEnv()
	s.Require().NoErrorf(err, "Missing env variable to configure suite")

	parsed, err := conn.ParsedConnectionFromStr(s.ConnStr)
	s.Require().NoErrorf(err, "connection string could not be parsed")
	s.Namespace = parsed.Namespace
	// suite.Token = suite.servicePrincipalToken()
	s.TagID = randomString("tag", 5)
	s.sbClient, err = azservicebus.NewClientFromConnectionString(s.ConnStr, nil)
	s.Require().NoError(err)
	s.sbAdminClient, err = azadmin.NewClientFromConnectionString(s.ConnStr, nil)
	s.Require().NoError(err)
}

func (s *SBSuite) ApplyPrefix(name string) string {
	return fmt.Sprintf("%s-%s", s.Prefix, name)
}

func (s *SBSuite) EnsureTopic(ctx context.Context, t *testing.T, name string) {
	topic, err := s.sbAdminClient.GetTopic(ctx, name, nil)
	require.NoError(t, err)
	if topic == nil {
		createResponse, err := s.sbAdminClient.CreateTopic(ctx, name, &azadmin.CreateTopicOptions{
			Properties: &azadmin.TopicProperties{},
		})
		require.NoError(t, err)
		t.Logf("topic created: %v", createResponse.Status)
		return
	}
	updateResponse, err := s.sbAdminClient.UpdateTopic(ctx, name, azadmin.TopicProperties{}, nil)
	require.NoError(t, err)
	t.Logf("topic updated: %v", updateResponse.Status)
}

func (s *SBSuite) EnsureTopicSubscription(ctx context.Context, t *testing.T, topicName, subscriptionName string) {
	sub, err := s.sbAdminClient.GetSubscription(ctx, topicName, subscriptionName, nil)
	require.NoError(t, err)
	if sub == nil {
		createResponse, err := s.sbAdminClient.CreateSubscription(ctx, topicName, subscriptionName, &azadmin.CreateSubscriptionOptions{
			Properties: &azadmin.SubscriptionProperties{
				LockDuration: to.Ptr("PT10S"),
			},
		})
		require.NoError(t, err)
		t.Logf("subscription created: %v", createResponse.Status)
		return
	}
	updateResponse, err := s.sbAdminClient.UpdateSubscription(ctx, topicName, subscriptionName, azadmin.SubscriptionProperties{
		LockDuration: to.Ptr("PT10S"),
	}, nil)
	require.NoError(t, err)
	t.Logf("subscription updated: %v", updateResponse.Status)
}

func (s *SBSuite) TearDownSuite() {
	p, _ := os.FindProcess(syscall.Getpid())
	p.Signal(syscall.SIGINT)
}

func (s *SBSuite) SetupTest() {

}

func (s *SBSuite) TearDownTest() {

}

func initTracing() (io.Closer, error) {
	cfg, err := config.FromEnv()
	if err != nil {
		return nil, err
	}
	jLogger := jaegerlog.NullLogger
	jMetricsFactory := metrics.NullFactory
	tracer, closer, err := cfg.NewTracer(config.Logger(jLogger), config.Metrics(jMetricsFactory))
	opentracing.SetGlobalTracer(tracer)
	if err != nil {
		return nil, err
	}
	return closer, nil
}
