package test

import (
	"context"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-amqp-common-go/v3/conn"
	rm "github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2017-05-10/resources"
	sbmgmt "github.com/Azure/azure-sdk-for-go/services/servicebus/mgmt/2017-04-01/servicebus"
	servicebus "github.com/Azure/azure-service-bus-go"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/suite"
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
	if err := godotenv.Load(); err != nil {
		suite.T().Log(err)
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
	suite.Token = suite.servicePrincipalToken()
	suite.Environment = azure.PublicCloud
	suite.TagID = randomString("tag", 10)

	if !suite.NoError(suite.ensureProvisioned(sbmgmt.SkuTierStandard)) {
		suite.FailNow("failed to ensure provisioned")
	}
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

func (suite *BaseSuite) servicePrincipalToken() *adal.ServicePrincipalToken {
	oauthConfig, err := adal.NewOAuthConfig(azure.PublicCloud.ActiveDirectoryEndpoint, suite.TenantID)
	if err != nil {
		suite.T().Fatal(err)
	}

	tokenProvider, err := adal.NewServicePrincipalToken(*oauthConfig,
		suite.ClientID,
		suite.ClientSecret,
		azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		suite.T().Fatal(err)
	}

	return tokenProvider
}

func (suite *BaseSuite) getRmGroupClient() *rm.GroupsClient {
	groupsClient := rm.NewGroupsClient(suite.SubscriptionID)
	groupsClient.Authorizer = autorest.NewBearerAuthorizer(suite.Token)
	return &groupsClient
}

func (suite *BaseSuite) getNamespaceClient() *sbmgmt.NamespacesClient {
	nsClient := sbmgmt.NewNamespacesClient(suite.SubscriptionID)
	nsClient.Authorizer = autorest.NewBearerAuthorizer(suite.Token)
	return &nsClient
}

func (suite *BaseSuite) ensureProvisioned(tier sbmgmt.SkuTier) error {
	groupsClient := suite.getRmGroupClient()
	_, err := groupsClient.CreateOrUpdate(context.Background(), suite.ResourceGroup, rm.Group{Location: &suite.Location})
	if err != nil {
		return err
	}

	nsClient := suite.getNamespaceClient()
	_, err = nsClient.Get(context.Background(), suite.ResourceGroup, suite.Namespace)
	if err != nil {
		ns := sbmgmt.SBNamespace{
			Sku: &sbmgmt.SBSku{
				Name: sbmgmt.SkuName(tier),
				Tier: tier,
			},
			Location: &suite.Location,
		}
		res, err := nsClient.CreateOrUpdate(context.Background(), suite.ResourceGroup, suite.Namespace, ns)
		if err != nil {
			return err
		}

		return res.WaitForCompletionRef(context.Background(), nsClient.Client)
	}

	return nil
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

// randomString generates a random Event Hub name tagged with the suite id
func (suite *BaseSuite) randomName(prefix string, length int) string {
	return randomString(prefix, length) + "-" + suite.TagID
}

// randomString generates a random string with prefix
func randomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}
