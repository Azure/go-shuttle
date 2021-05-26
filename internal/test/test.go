package test

import (
	"context"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-amqp-common-go/v3/conn"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	_ "github.com/devigned/tab/opentracing"
	"github.com/joho/godotenv"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/suite"
	"github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"
)

type (
	// BaseSuite encapsulates a end to end test of Service Bus with build up and tear down of all SB resources
	BaseSuite struct {
		suite.Suite
		TenantID       string
		SubscriptionID string
		ClientID       string
		ClientSecret   string
		ConnStr        string
		Location       string
		Namespace      string
		ResourceGroup  string
		Token          *adal.ServicePrincipalToken
		Environment    azure.Environment
		TagID          string
		closer         io.Closer
	}
)

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyz123456789")
)

func init() {
	rand.Seed(time.Now().Unix())
}

// SetupSuite prepares the test suite and provisions a standard Service Bus Namespace
func (suite *BaseSuite) SetupSuite() {
	if err := godotenv.Load("../.env"); err != nil {
		suite.T().Log(err)
	}
	if os.Getenv("TRACING") == "1" {
		_, err := initTracing()
		if err != nil {
			suite.FailNow("failed to initialize tracing %s", err)
		}
	}

	setFromEnv := func(key string, target *string) {
		v := os.Getenv(key)
		if v == "" {
			suite.FailNowf("missing environment variable", "%q required for integration tests.", key)
		}
		*target = v
	}

	setFromEnv("AZURE_TENANT_ID", &suite.TenantID)
	setFromEnv("AZURE_SUBSCRIPTION_ID", &suite.SubscriptionID)
	setFromEnv("AZURE_CLIENT_ID", &suite.ClientID)
	setFromEnv("AZURE_CLIENT_SECRET", &suite.ClientSecret)
	setFromEnv("SERVICEBUS_CONNECTION_STRING", &suite.ConnStr)
	setFromEnv("TEST_RESOURCE_GROUP", &suite.ResourceGroup)

	// TODO: automatically infer the location from the resource group, if it's not specified.
	// https://github.com/Azure/azure-service-bus-go/issues/40
	setFromEnv("TEST_LOCATION", &suite.Location)

	parsed, err := conn.ParsedConnectionFromStr(suite.ConnStr)
	if !suite.NoError(err) {
		suite.FailNowf("connection string could not be parsed", "Connection String: %q", suite.ConnStr)
	}
	suite.Namespace = parsed.Namespace
	// suite.Token = suite.servicePrincipalToken()
	suite.Environment = azure.PublicCloud
	suite.TagID = randomString("tag", 5)
}

// TearDownSuite destroys created resources during the run of the suite. In particular it deletes the topics that were created
// for the duration of this test.
func (suite *BaseSuite) TearDownSuite() {
	if suite.closer != nil {
		_ = suite.closer.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	suite.deleteAllTaggedTopics(ctx)
}

func (suite *BaseSuite) deleteAllTaggedTopics(ctx context.Context) {
	ns := suite.GetNewNamespace()
	tm := ns.NewTopicManager()

	topics, err := tm.List(ctx)
	if err != nil {
		suite.T().Fatal(err)
	}

	for _, topic := range topics {
		if strings.HasSuffix(topic.Name, suite.TagID) {
			err := tm.Delete(ctx, topic.Name)
			if err != nil {
				suite.T().Fatal(err)
			}
		}
	}
}

// GetNewNamespace assumes that a ServiceBus namespace has been created ahead of time
func (suite *BaseSuite) GetNewNamespace(opts ...servicebus.NamespaceOption) *servicebus.Namespace {
	ns, err := servicebus.NewNamespace(append(opts, servicebus.NamespaceWithConnectionString(suite.ConnStr))...)
	if err != nil {
		suite.T().Fatal(err)
	}
	return ns
}

// EnsureTopic checks if the topic exists and creates one if it doesn't
func (suite *BaseSuite) EnsureTopic(ctx context.Context, name string) (*servicebus.TopicEntity, error) {
	ns := suite.GetNewNamespace()
	tm := ns.NewTopicManager()

	te, err := tm.Get(ctx, name)
	if err == nil {
		return te, nil
	}

	return tm.Put(ctx, name)
}

// EnsureQueue checks if the queue exists and creates one if it doesn't
func (suite *BaseSuite) EnsureQueue(ctx context.Context, name string) (*servicebus.QueueEntity, error) {
	ns := suite.GetNewNamespace()
	qm := ns.NewQueueManager()

	qe, err := qm.Get(ctx, name)
	if err == nil {
		return qe, nil
	}

	return qm.Put(ctx, name)
}

// randomString generates a random string with prefix
func randomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
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
