PLUGIN_NAME = lizardfsdocker/lizardfs-volume-plugin
PLUGIN_TAG ?= latest
TRAVIS_BUILD_NUMBER ?= local

all: clean rootfs create

clean:
	@echo "### rm ./plugin"
	@rm -rf ./plugin

config:
	@echo "### copy config.json to ./plugin/"
	@mkdir -p ./plugin
	@cp config.json ./plugin/

rootfs: config
	@echo "### docker build: rootfs image with"
	@docker build -t ${PLUGIN_NAME}:rootfs .
	@echo "### create rootfs directory in ./plugin/rootfs"
	@mkdir -p ./plugin/rootfs
	@docker create --name tmp ${PLUGIN_NAME}:rootfs
	@docker export tmp | tar -x -C ./plugin/rootfs
	@docker rm -vf tmp

create:
	@echo "### remove existing plugin ${PLUGIN_NAME}:${PLUGIN_TAG} if exists"
	@docker plugin rm -f ${PLUGIN_NAME}:${PLUGIN_TAG} || true
	@docker plugin rm -f ${PLUGIN_NAME}:${TRAVIS_BUILD_NUMBER} || true
	@echo "### create new plugin ${PLUGIN_NAME}:${PLUGIN_TAG} from ./plugin"
	@docker plugin create ${PLUGIN_NAME}:${PLUGIN_TAG} ./plugin
	@docker plugin create ${PLUGIN_NAME}:${TRAVIS_BUILD_NUMBER} ./plugin

enable:
	@echo "### enable plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker plugin enable ${PLUGIN_NAME}:${PLUGIN_TAG}

disable:
	@echo "### disable plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker plugin disable ${PLUGIN_NAME}:${PLUGIN_TAG}

push:  clean rootfs create
	@echo "### push plugin ${PLUGIN_NAME}:${PLUGIN_TAG}"
	@docker plugin push ${PLUGIN_NAME}:${TRAVIS_BUILD_NUMBER}
	@docker plugin push ${PLUGIN_NAME}:${PLUGIN_TAG}
