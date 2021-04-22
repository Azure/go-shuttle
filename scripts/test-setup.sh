#!/usr/bin/env bash

set -euo pipefail

echo "creating RG"
az group create \
--name ${TEST_RESOURCE_GROUP} \
--location ${TEST_LOCATION} \
--subscription ${AZURE_SUBSCRIPTION_ID} \
-o none

echo "create managed identity"
MANAGED_IDENTITY_PRINCIPAL_ID=$(az identity create \
--name "${SERVICEBUS_NAMESPACE_NAME}_id" \
--subscription ${AZURE_SUBSCRIPTION_ID} \
-g ${TEST_RESOURCE_GROUP} \
--query principalId \
-o tsv)

MANAGED_IDENTITY_CLIENT_ID=$(az identity show \
--name "${SERVICEBUS_NAMESPACE_NAME}_id" \
--subscription ${AZURE_SUBSCRIPTION_ID} \
-g ${TEST_RESOURCE_GROUP} \
--query clientId \
-o tsv)

MANAGED_IDENTITY_RESOURCE_ID="/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourcegroups/${TEST_RESOURCE_GROUP}/providers/Microsoft.ManagedIdentity/userAssignedIdentities/${SERVICEBUS_NAMESPACE_NAME}_id"

echo "creating ACR"
az acr create \
--name ${REGISTRY_NAME} \
--location ${TEST_LOCATION} \
--resource-group ${TEST_RESOURCE_GROUP} \
--subscription ${AZURE_SUBSCRIPTION_ID} \
--admin-enabled true \
--sku Basic \
-o none

echo "getting ACR credentials"
REGISTRY=$(az acr show --name ${REGISTRY_NAME} --subscription ${AZURE_SUBSCRIPTION_ID} --query loginServer  -o tsv)
REGISTRY_USER=$(az acr credential show --name ${REGISTRY_NAME} --subscription ${AZURE_SUBSCRIPTION_ID} --query username -o tsv)
REGISTRY_PASSWORD=$(az acr credential show --name ${REGISTRY_NAME} --subscription ${AZURE_SUBSCRIPTION_ID} --query "passwords | [0].value" -o tsv)

echo "create servicebus namespace"
SERVICEBUS_ID=$(az servicebus namespace create \
--name ${SERVICEBUS_NAMESPACE_NAME} \
-l ${TEST_LOCATION} \
-g ${TEST_RESOURCE_GROUP} \
--subscription ${AZURE_SUBSCRIPTION_ID} \
--sku premium \
--query id \
-o tsv)

echo "get servicebus connection string"
SERVICEBUS_CONNECTION_STRING=$(az servicebus namespace authorization-rule keys list \
--resource-group ${TEST_RESOURCE_GROUP} \
--namespace-name ${SERVICEBUS_NAMESPACE_NAME} \
--subscription ${AZURE_SUBSCRIPTION_ID} \
--name "RootManageSharedAccessKey" \
--query primaryConnectionString \
-o tsv)

echo "assign servicebus role to identity"
az role assignment create \
--assignee-object-id "${MANAGED_IDENTITY_PRINCIPAL_ID}" \
--scope "${SERVICEBUS_ID}" \
--role "Azure Service Bus Data Owner" \
-o none

echo "adding config to .env"
DOTENV=$1
sed -i .bak "s|MANAGED_IDENTITY_CLIENT_ID=.*|MANAGED_IDENTITY_CLIENT_ID=${MANAGED_IDENTITY_CLIENT_ID}|g" ${DOTENV}
sed -i .bak "s|MANAGED_IDENTITY_PRINCIPAL_ID=.*|MANAGED_IDENTITY_PRINCIPAL_ID=${MANAGED_IDENTITY_PRINCIPAL_ID}|g" ${DOTENV}
sed -i .bak "s|MANAGED_IDENTITY_RESOURCE_ID=.*|MANAGED_IDENTITY_RESOURCE_ID=${MANAGED_IDENTITY_RESOURCE_ID}|g" ${DOTENV}
sed -i .bak "s|REGISTRY_USER=.*|REGISTRY_USER=${REGISTRY_USER}|g" ${DOTENV}
sed -i .bak "s|REGISTRY_PASSWORD=.*|REGISTRY_PASSWORD=${REGISTRY_PASSWORD}|g" ${DOTENV}
sed -i .bak "s|REGISTRY=.*|REGISTRY=${REGISTRY}|g" ${DOTENV}
sed -i .bak "s|SERVICEBUS_ID=.*|SERVICEBUS_ID=${SERVICEBUS_ID}|g" ${DOTENV}
sed -i .bak "s|SERVICEBUS_CONNECTION_STRING=.*|SERVICEBUS_CONNECTION_STRING=${SERVICEBUS_CONNECTION_STRING}|g" ${DOTENV}

rm "${DOTENV}.bak"