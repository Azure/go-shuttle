SCRIPTPATH=$(shell cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P)

ifneq (,$(wildcard ./.env))
    $(info found env file)
    include .env
    export
endif

ENVFILE=${SCRIPTPATH}/.env

IMAGE?=${REGISTRY}/pubsubtest
LOG_DIRECTORY?=/aci/logs/localrun_$(shell date -u +"%FT%H%M%S%Z")
ACI_CONTAINER_NAME?=tester
export

.PHONY: test-setup
test-setup:
	scripts/test-setup.sh "${ENVFILE}"

.PHONY: test-env
test-env:
	scripts/test-env.sh "${ENVFILE}"

.PHONY: cleanup-test-setup
cleanup-test-setup:
	az group delete --name ${TEST_RESOURCE_GROUP} --subscription ${AZURE_SUBSCRIPTION_ID}

build-test-image:
	echo "envfile : ${ENVFILE}"
	echo "REGISTRY : ${REGISTRY}"

	docker build -t ${IMAGE} .

push-test-image:
	@docker login -u ${REGISTRY_USER} -p ${REGISTRY_PASSWORD} ${REGISTRY}
	docker push ${IMAGE}


test-aci: clean-aci scripts/containergroup.yaml
	containerId=$$(az container create --file scripts/containergroup.yaml \
	--name "${ACI_CONTAINER_NAME}" \
	--resource-group ${TEST_RESOURCE_GROUP} \
	--subscription ${AZURE_SUBSCRIPTION_ID} \
	--environment-variables SUITE=${SUITE} \
	--verbose \
	--query id -o tsv); \
	./scripts/wait-aci.sh $${containerId}

shell-aci: clean-aci
	az container create --file scripts/containergroup.yaml \
	--resource-group ${TEST_RESOURCE_GROUP} \
	--subscription ${AZURE_SUBSCRIPTION_ID} \
	--command-line "/bin/bash"; \
	az container attach --name "pubsubtester" --resource-group "${TEST_RESOURCE_GROUP}"

scripts/containergroup.yaml:
	envsubst < scripts/containergroup.template.yaml > scripts/containergroup.yaml

clean-aci:
	az container delete \
	--resource-group ${TEST_RESOURCE_GROUP} \
	--name ${ACI_CONTAINER_NAME} \
	--subscription ${AZURE_SUBSCRIPTION_ID} \
	--yes

integration: build-test-image push-test-image test-aci

integration-compose: build-test-image
	@docker-compose --env-file "${ENVFILE}" up

integration-local:
	LOG_DIRECTORY=. ./run-integration.sh TestConnectionString/TestCreate*

integration-pipeline: scripts/containergroup.yaml
	SUITE=$$(echo "${SUITE}" | tr '[:upper:]' '[:lower:]') \
	containerId=$$(az container create --file scripts/containergroup.yaml \
	--name "bld${ACI_CONTAINER_NAME}-${SUITE}" \
	--resource-group ${TEST_RESOURCE_GROUP} \
	--subscription ${AZURE_SUBSCRIPTION_ID} \
	--verbose \
	--query id -o tsv); \
	./scripts/wait-aci.sh $${containerId}

integration-clean-aci:
	SUITE=$$(echo "${SUITE}" | tr '[:upper:]' '[:lower:]') \
	az rest \
	--method delete \
	--uri "/subscriptions/{subscriptionId}/resourceGroups/${TEST_RESOURCE_GROUP}/providers/Microsoft.ContainerInstance/containerGroups/bld${ACI_CONTAINER_NAME}-${SUITE}?api-version=2019-12-01" \
	--subscription 2b03bfb8-e885-4566-a62a-909a11d71692

download-junit:
	az storage file download-batch \
	--account-name ${STORAGE_ACCOUNT_NAME} \
	--account-key ${STORAGE_ACCOUNT_KEY} \
	--source "acilogs" \
	--pattern "${LOG_DIRECTORY}/*.junit.xml" \
	--dest . \
	--output none


