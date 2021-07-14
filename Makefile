SCRIPTPATH=$(shell cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P)

ifneq (,$(wildcard ./.env))
    $(info found env file)
    include .env
    export
endif

ENVFILE=${SCRIPTPATH}/.env

IMAGE?=${REGISTRY}/pubsubtest
export

.PHONY: test-setup
test-setup:
	scripts/test-setup.sh "${ENVFILE}"

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
	--resource-group ${TEST_RESOURCE_GROUP} \
	--subscription ${AZURE_SUBSCRIPTION_ID} \
	--verbose \
	--query id -o tsv) ;\
	az container logs --ids $${containerId} --follow

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
	--name pubsubtester \
	--subscription ${AZURE_SUBSCRIPTION_ID} \
	--yes

integration: build-test-image push-test-image test-aci

integration-compose: build-test-image
	@docker-compose --env-file "${ENVFILE}" up

integration-local:
	./run-integration.sh -run TestConnectionString


