op-develop: op-build op-compose

## build prometheus image

PROMETHEUS_IMAGE := "op-prometheus:develop"

op-build:
	docker build -f op.Dockerfile --tag ${PROMETHEUS_IMAGE} .

## start prometheus

op-compose:
	PROMETHEUS_IMAGE=${PROMETHEUS_IMAGE} \
		docker compose -f op-develop/docker-compose.yml up --force-recreate --remove-orphans

## update prometheus version

op-list-changes:
	grep -rnw '.' -e 'OP_CHANGES.md'
